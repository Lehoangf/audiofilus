package main

import (
	"log"
	"os/exec"
	"sync"
	"time"
)

// controlDebounce là khoảng cách tối thiểu giữa hai lần nhận cùng một nút. Rung
// cơ học của phím F1/F2 thường < 5 ms, nhưng EV_KEY đã lọc phần lớn; ngưỡng này
// chủ yếu chặn trường hợp một nút xuất hiện trên cả event3 lẫn event0.
const controlDebounce = 180 * time.Millisecond

// injector dùng chung cho mọi hành động RetroArch: tạo thiết bị uinput ảo một
// lần, rồi chỉ ghi sự kiện nhấn/phím nhả. Đóng fd (khi daemon thoát) sẽ tự xóa
// thiết bị ảo, không để lại dấu vết trên hệ thống.
var (
	injectorOnce sync.Once
	injector     *keyInjector
	injectorErr  error
)

// RunControl chạy hành động đã map cho một nút phụ. Hàm được gọi từ vòng lặp
// evdev, nên nó phải trả về nhanh: mọi lệnh shell đều đi goroutine riêng hoặc
// có deadline, và không bao giờ giữ khóa khi chạy lệnh ngoài.
func (h *HardwareManager) RunControl(btn ButtonKind, cfg *Config) {
	action := cfg.Controls.ControlAction(btn)
	if action == ActionNone {
		return
	}

	h.mu.Lock()
	now := time.Now()
	if now.Sub(h.lastPress[btn]) < controlDebounce {
		h.mu.Unlock()
		return
	}
	h.lastPress[btn] = now
	turnLEDsOn := false
	if action == ActionLEDToggle {
		h.ledsOn = !h.ledsOn
		turnLEDsOn = h.ledsOn
	}
	h.mu.Unlock()

	log.Printf("[BUTTON] %s -> %s", btn, action)
	switch action {
	case ActionMute:
		toggleMute()
	case ActionScreenshot:
		takeScreenshot()
	case ActionBrightnessDown:
		stepBrightness(-10)
	case ActionBrightnessUp:
		stepBrightness(10)
	case ActionLEDToggle:
		if turnLEDsOn {
			restoreAllLEDs(nil)
		} else {
			turnOffAllLEDs()
		}
	case ActionRetroArchFF, ActionRetroArchSave, ActionRetroArchMenu, ActionQuickMenu:
		tapRetroArchKey(cfg.Controls.RetroArchKeys.RetroArchKey(action))
	case ActionExitES:
		exitEmulationStation()
	case ActionCustom:
		if cmd := cfg.Controls.ControlCommand(btn); cmd != "" {
			runShellCommand(cmd)
		} else {
			log.Printf("[BUTTON] %s map 'custom' nhưng chưa có lệnh trong config.json", btn)
		}
	}
}

// toggleMute chỉ bật/tắt mute phần mềm của sink. Không đặt âm lượng về 0% vì
// Knulli lưu audio.volume từ chính mức sink lúc tắt máy (xem README).
func toggleMute() {
	target := "@DEFAULT_SINK@"
	if sinkMuted(target) {
		runCmd("pactl", "set-sink-mute", target, "0")
		log.Printf("[BUTTON] Đã bỏ mute âm thanh")
		return
	}
	runCmd("pactl", "set-sink-mute", target, "1")
	log.Printf("[BUTTON] Đã mute âm thanh")
}

// takeScreenshot dùng tiện ích có sẵn của Knulli/Batocera; không tự đụng vào
// framebuffer vì knulli-screenshot đã biết đúng thư mục và tên file.
func takeScreenshot() {
	for _, bin := range []string{"knulli-screenshot", "batocera-screenshot"} {
		if _, err := exec.LookPath(bin); err == nil {
			if runCmd(bin) {
				log.Printf("[BUTTON] Đã chụp màn hình bằng %s", bin)
				return
			}
		}
	}
	log.Printf("[BUTTON] Không tìm thấy knulli-screenshot / batocera-screenshot")
}

// stepBrightness tăng/giảm độ sáng hiện tại theo từng bước nhỏ, kẹp trong
// khoảng 0-100 của knulli-brightness.
func stepBrightness(delta int) {
	cur := getBrightness()
	next := cur + delta
	if next < 0 {
		next = 0
	} else if next > 100 {
		next = 100
	}
	if next == cur {
		return
	}
	setBrightness(next)
	log.Printf("[BUTTON] Độ sáng %d%% -> %d%%", cur, next)
}

// initKeyInjector tạo bàn phím ảo ngay lúc daemon khởi động. Làm sớm để
// RetroArch (không theo dõi nóng thiết bị input trong mọi bản firmware) vẫn kịp
// nhận ra bàn phím trước khi người dùng bấm nút.
func initKeyInjector() {
	injectorOnce.Do(func() {
		injector, injectorErr = newKeyInjector([]uint16{
			keyF1, keyF2, keyF, keyHomepage, keyMenu, keyHome,
		})
		if injectorErr != nil {
			log.Printf("[BUTTON] Không tạo được bàn phím ảo uinput: %v", injectorErr)
		} else {
			log.Printf("[BUTTON] Đã tạo bàn phím ảo cho các hành động RetroArch")
		}
	})
}

// tapRetroArchKey tiêm một cú nhấn phím ngắn tới RetroArch đang chạy. Daemon đọc
// input thụ động nên không "ăn" được nút vật lý thay ứng dụng; uinput là cách
// duy nhất để gửi lệnh tới RetroArch mà không cần X11 hay sửa cấu hình RetroArch.
func tapRetroArchKey(code uint16) {
	if code == 0 {
		log.Printf("[BUTTON] Mã phím RetroArch không hợp lệ, bỏ qua")
		return
	}
	initKeyInjector()
	if injectorErr != nil || injector == nil {
		return
	}
	if err := injector.Tap(code); err != nil {
		log.Printf("[BUTTON] Không tiêm được phím %d: %v", code, err)
		return
	}
	log.Printf("[BUTTON] Đã gửi phím %d tới RetroArch", code)
}

// exitEmulationStation yêu cầu EmulationStation thoát để Knulli đưa về menu /
// khởi động lại giao diện. Dùng -x (khớp đúng tên tiến trình) thay vì -f để
// không bao giờ tự giết chính tiến trình gọi lệnh này.
func exitEmulationStation() {
	if runCmd("pkill", "-TERM", "-x", "emulationstation") {
		log.Printf("[BUTTON] Đã yêu cầu EmulationStation thoát")
		return
	}
	runCmd("killall", "-q", "emulationstation")
}
