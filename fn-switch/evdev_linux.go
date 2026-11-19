//go:build linux

package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// EVIOCGSW và EVIOCGBIT(EV_KEY, 32): đọc bitmap switch / phím của một thiết bị.
const (
	eviocgsW32 = 0x8020451b // _IOR('E', 0x1b, char[32])
	eviocgbitK = 0x80204521 // _IOR('E', 0x21, char[32])
)

// InputDeviceInfo chứa thông tin thiết bị từ /proc/bus/input/devices.
type InputDeviceInfo struct {
	Name     string
	Path     string // /dev/input/eventX
	HasSW    bool
	SwBitmap string
}

// ListInputDevices duyệt qua /proc/bus/input/devices để lấy danh sách thiết bị.
func ListInputDevices() ([]InputDeviceInfo, error) {
	f, err := os.Open("/proc/bus/input/devices")
	if err != nil {
		matches, _ := filepath.Glob("/dev/input/event*")
		var list []InputDeviceInfo
		for _, m := range matches {
			list = append(list, InputDeviceInfo{Name: filepath.Base(m), Path: m})
		}
		return list, nil
	}
	defer f.Close()

	var list []InputDeviceInfo
	var cur InputDeviceInfo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			if cur.Path != "" {
				list = append(list, cur)
			}
			cur = InputDeviceInfo{}
		case strings.HasPrefix(line, "N: Name="):
			cur.Name = strings.Trim(strings.TrimPrefix(line, "N: Name="), `"`)
		case strings.HasPrefix(line, "H: Handlers="):
			parts := strings.Fields(line)
			for _, p := range parts {
				if strings.HasPrefix(p, "event") {
					cur.Path = "/dev/input/" + p
				}
			}
		case strings.HasPrefix(line, "B: SW="):
			cur.HasSW = true
			cur.SwBitmap = strings.TrimPrefix(line, "B: SW=")
		}
	}
	if cur.Path != "" {
		list = append(list, cur)
	}
	return list, sc.Err()
}

// ReadInputEvent đọc 1 sự kiện từ /dev/input/eventX.
func ReadInputEvent(f *os.File) (evType uint16, evCode uint16, evVal int32, err error) {
	var buf [inputEventSize]byte
	if _, err = io.ReadFull(f, buf[:]); err != nil {
		return 0, 0, 0, err
	}
	evType = binary.LittleEndian.Uint16(buf[16:18])
	evCode = binary.LittleEndian.Uint16(buf[18:20])
	evVal = int32(binary.LittleEndian.Uint32(buf[20:24]))
	return evType, evCode, evVal, nil
}

// getSwitchState đọc trạng thái switch hiện tại qua ioctl EVIOCGSW.
func getSwitchState(fd uintptr, code int) (bool, error) {
	var buf [32]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, eviocgsW32, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return false, errno
	}
	byteIdx := code / 8
	bitIdx := code % 8
	if byteIdx < len(buf) {
		isOn := ((buf[byteIdx] >> bitIdx) & 1) == 1
		return isOn, nil
	}
	return false, nil
}

// deviceHasButtons kiểm tra xem một thiết bị có sinh ra EV_KEY cho F1/F2/Home
// không. Dùng EVIOCGBIT thay vì đoán theo tên: trên Brick Pro các phím này có
// thể nằm ở event3 (controller), event0 (gpio-keys) hoặc cả hai, tùy bản Knulli.
func deviceHasButtons(path string) bool {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(fd)

	var bits [32]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), eviocgbitK, uintptr(unsafe.Pointer(&bits[0])))
	if errno != 0 {
		return false
	}
	has := func(code uint16) bool {
		idx := int(code / 8)
		return idx < len(bits) && bits[idx]&(1<<(code%8)) != 0
	}
	return has(keyF1) || has(keyF2) || has(keyHomepage) || has(keyMenu) || has(keyHome)
}

// FindMatchingDevice tìm thiết bị switch FN theo cấu hình, rơi về quét tự động.
func FindMatchingDevice(cfg *Config) (string, error) {
	if cfg.Device.EventPath != "auto" && cfg.Device.EventPath != "" {
		if _, err := os.Stat(cfg.Device.EventPath); err == nil {
			return cfg.Device.EventPath, nil
		}
	}

	devs, err := ListInputDevices()
	if err != nil {
		return "", err
	}

	nameFilter := strings.ToLower(cfg.Device.DeviceName)
	if nameFilter != "" {
		for _, d := range devs {
			if strings.Contains(strings.ToLower(d.Name), nameFilter) {
				return d.Path, nil
			}
		}
	}
	for _, d := range devs {
		if d.HasSW {
			log.Printf("[DISCOVERY] Tìm thấy thiết bị có Switch phần cứng: %s (%s)", d.Path, d.Name)
			return d.Path, nil
		}
	}
	for _, d := range devs {
		lower := strings.ToLower(d.Name)
		if strings.Contains(lower, "brick") || strings.Contains(lower, "controller") || strings.Contains(lower, "trimui") {
			return d.Path, nil
		}
	}
	if len(devs) > 0 {
		return devs[0].Path, nil
	}
	return "", fmt.Errorf("không tìm thấy thiết bị input phù hợp")
}

// collectInputPaths trả về danh sách thiết bị cần đọc: thiết bị công tắc FN
// cộng với mọi thiết bị có phím F1/F2/Home. Loại trùng để không xử lý một lần
// bấm hai lần, nhưng vẫn cho phép Home nằm trên một event device khác.
func collectInputPaths(cfg *Config) []string {
	var paths []string
	seen := make(map[string]bool)
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	if primary, err := FindMatchingDevice(cfg); err == nil {
		add(primary)
	}
	devs, _ := ListInputDevices()
	for _, d := range devs {
		if !seen[d.Path] && deviceHasButtons(d.Path) {
			add(d.Path)
		}
	}
	if len(paths) == 0 {
		// Lần cuối cùng: để cơ chế chờ thiết bị bên dưới tự xử lý boot sớm.
		if cfg.Device.EventPath != "" && cfg.Device.EventPath != "auto" {
			add(cfg.Device.EventPath)
		} else {
			add("/dev/input/event3")
		}
	}
	return paths
}

// rawEvent là một sự kiện thô kèm đường dẫn thiết bị để log khi cần.
type rawEvent struct {
	path string
	t    uint16
	code uint16
	val  int32
}

// readDevice đọc liên tục một thiết bị và đẩy sự kiện vào kênh chung. Khi mất
// thiết bị (rút OTG, boot lại input) nó tự mở lại sau một khoảng ngắn.
func readDevice(path string, out chan<- rawEvent, stop <-chan struct{}) {
	failedOpens := 0
	for {
		select {
		case <-stop:
			return
		default:
		}
		f, err := os.Open(path)
		if err != nil {
			failedOpens++
			if failedOpens == 1 || failedOpens%10 == 0 {
				log.Printf("[EVDEV] Chưa mở được %s (lần %d): %v", path, failedOpens, err)
			}
			time.Sleep(2 * time.Second)
			continue
		}
		if failedOpens > 0 {
			log.Printf("[EVDEV] Đã kết nối lại được %s", path)
			failedOpens = 0
		}
		for {
			t, code, val, err := ReadInputEvent(f)
			if err != nil {
				break
			}
			select {
			case out <- rawEvent{path: path, t: t, code: code, val: val}:
			case <-stop:
				_ = f.Close()
				return
			}
		}
		_ = f.Close()
		time.Sleep(500 * time.Millisecond)
	}
}

// RunProbeMode mở tất cả /dev/input/event* để người dùng gạt thử FN switch và
// bấm thử các nút phụ, rồi lưu mã tìm được vào config.json.
func RunProbeMode(configPath string) {
	devs, err := ListInputDevices()
	if err != nil || len(devs) == 0 {
		log.Fatalf("[PROBE] Không tìm thấy thiết bị input nào trong /proc/bus/input/devices")
	}

	fmt.Println("==================================================================")
	fmt.Println("             TRIMUI BRICK PRO - FN SWITCH PROBE MODE              ")
	fmt.Println("==================================================================")
	fmt.Println("Đang lắng nghe tất cả các cổng /dev/input/event*...")
	fmt.Println("👉 GẠT công tắc FN hoặc BẤM F1 / F2 / Home trên máy...")
	fmt.Println("------------------------------------------------------------------")

	type probeEvent struct {
		dev     InputDeviceInfo
		t, code uint16
		val     int32
	}
	eventChan := make(chan probeEvent, 10)

	for _, d := range devs {
		f, err := os.Open(d.Path)
		if err != nil {
			continue
		}
		go func(dev InputDeviceInfo, file *os.File) {
			defer file.Close()
			for {
				t, code, val, err := ReadInputEvent(file)
				if err != nil {
					return
				}
				if t == evSw || t == evKey {
					eventChan <- probeEvent{dev: dev, t: t, code: code, val: val}
				}
			}
		}(d, f)
	}

	for ev := range eventChan {
		typeName := "EV_SW (Switch)"
		if ev.t == evKey {
			typeName = "EV_KEY (Button)"
		}
		label := ""
		if ev.t == evKey {
			if btn, ok := buttonForKey(ev.code); ok {
				label = fmt.Sprintf(" -> nút %s", btn)
			}
		}
		fmt.Printf("\n🎯 PHÁT HIỆN SỰ KIỆN PHẦN CỨNG!\n")
		fmt.Printf("   Thiết bị: %s (%s)\n", ev.dev.Path, ev.dev.Name)
		fmt.Printf("   Loại sự kiện: 0x%02x (%s)\n", ev.t, typeName)
		fmt.Printf("   Mã code: %d (0x%04x)%s\n", ev.code, ev.code, label)
		fmt.Printf("   Giá trị (Value): %d\n", ev.val)

		cfg, _ := LoadConfig(configPath)
		cfg.Device.EventPath = ev.dev.Path
		cfg.Device.DeviceName = ev.dev.Name
		if ev.t == evSw {
			// Chỉ gạt switch mới ghi đè cấu hình FN; bấm nút thì giữ nguyên
			// cấu hình switch hiện có.
			cfg.Device.EventType = ev.t
			cfg.Device.EventCode = int(ev.code)
		}
		if err := SaveConfig(configPath, cfg); err == nil {
			fmt.Printf("\n✅ ĐÃ TỰ ĐỘNG CẬP NHẬT VÀO: %s\n", configPath)
			fmt.Println("Bây giờ bạn có thể khởi động fn-daemon bình thường!")
		} else {
			fmt.Printf("❌ Lỗi lưu config: %v\n", err)
		}
		return
	}
}

// ListenEvents mở vòng lặp đọc sự kiện và áp dụng trạng thái. Cùng một luồng
// xử lý cả công tắc FN (EV_SW) lẫn ba nút phụ (EV_KEY), nhờ đó hành động map
// trong config.json cuối cùng cũng có hiệu lực.
func ListenEvents(cfg *Config, hw *HardwareManager) error {
	paths := collectInputPaths(cfg)
	if len(paths) == 0 {
		return fmt.Errorf("hết thời gian chờ: không tìm thấy thiết bị input")
	}

	fnCode := cfg.Device.EventCode
	if fnCode < 0 {
		fnCode = fnSwitchCode
	}
	fnType := cfg.Device.EventType
	if fnType == 0 {
		fnType = evSw
	}

	// 1. Đọc trạng thái vật lý ban đầu của công tắc ngay khi mở.
	fnPath := paths[0]
	if primary, err := FindMatchingDevice(cfg); err == nil {
		fnPath = primary
	}
	if f, err := os.Open(fnPath); err == nil {
		if initialOn, errSw := getSwitchState(f.Fd(), fnCode); errSw == nil {
			if cfg.Device.Invert {
				initialOn = !initialOn
			}
			log.Printf("[EVDEV] Trạng thái FN ban đầu (Code=%d): ON=%v", fnCode, initialOn)
			hw.ApplySwitchState(initialOn, cfg)
		} else {
			log.Printf("[EVDEV] Không đọc được EVIOCGSW ban đầu (%v), mặc định OFF", errSw)
			hw.ApplySwitchState(false, cfg)
		}
		_ = f.Close()
	} else {
		log.Printf("[EVDEV] Không mở được %s để đọc trạng thái FN: %v", fnPath, err)
		hw.ApplySwitchState(false, cfg)
	}

	// 2. Khởi động một reader cho mỗi thiết bị, tất cả cùng đẩy vào một kênh.
	events := make(chan rawEvent, 64)
	stop := make(chan struct{})
	for _, p := range paths {
		go readDevice(p, events, stop)
		log.Printf("[EVDEV] Đang lắng nghe %s", p)
	}
	log.Printf("[EVDEV] Sẵn sàng: FN(type=%d, code=%d, invert=%v), nút F1/F2/Home",
		fnType, fnCode, cfg.Device.Invert)

	// 3. Kênh không bao giờ đóng: daemon chạy đến khi nhận SIGTERM.
	for ev := range events {
		if ev.t == fnType && int(ev.code) == fnCode {
			isOn := ev.val == 1
			if cfg.Device.Invert {
				isOn = !isOn
			}
			log.Printf("[INPUT] FN switch (Type=0x%02x, Code=%d, Val=%d) -> ON=%v",
				ev.t, ev.code, ev.val, isOn)
			hw.ApplySwitchState(isOn, cfg)
			continue
		}

		// Chỉ xử lý cú nhấn đầu tiên. value=2 là auto-repeat, value=0 là nhả.
		if ev.t == evKey && ev.val == 1 {
			if btn, ok := buttonForKey(ev.code); ok {
				log.Printf("[INPUT] Nút %s (Code=%d) từ %s", btn, ev.code, ev.path)
				hw.RunControl(btn, cfg)
			}
		}
	}
	close(stop)
	return nil
}
