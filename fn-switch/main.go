package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

const (
	defaultKnulliConfig = "/userdata/system/fn_switch/config.json"
	localConfig         = "config.json"
)

func resolveConfigPath(customPath string) string {
	if customPath != "" {
		return customPath
	}
	if _, err := os.Stat(defaultKnulliConfig); err == nil {
		return defaultKnulliConfig
	}
	return localConfig
}

func main() {
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		_ = os.Setenv("XDG_RUNTIME_DIR", "/var/run")
	}
	if os.Getenv("PULSE_SERVER") == "" {
		_ = os.Setenv("PULSE_SERVER", "unix:/var/run/pulse/native")
	}
	configFlag := flag.String("config", "", "Đường dẫn file cấu hình JSON")
	probeFlag := flag.Bool("probe", false, "Chạy chế độ dò tìm phần cứng (Probe Mode) khi gạt FN")
	statusFlag := flag.Bool("status", false, "Xem thông số phần cứng hiện tại của máy")
	audioFlag := flag.Bool("audio", false, "Kiểm tra và sửa đường ra âm thanh (loa trong)")
	toneFlag := flag.Bool("tone", false, "Phát âm thử 440 Hz sau khi sửa âm thanh (dùng với -audio)")
	dacFlag := flag.Int("dac", 0, "Thử mức DAC volume khác (0-255) cho lần chạy này (dùng với -audio)")
	flag.Parse()

	configPath := resolveConfigPath(*configFlag)

	// 1. Chế độ xem trạng thái phần cứng hiện tại
	if *statusFlag {
		fmt.Println("=== TRẠNG THÁI PHẦN CỨNG HIỆN TẠI (TRIMUI BRICK PRO) ===")
		fmt.Printf("CPU Governor:    %s\n", getCPUGovernor())
		fmt.Printf("Độ sáng màn hình: %d%%\n", getBrightness())
		fmt.Printf("Âm lượng:        %d%%\n", getVolume())
		fmt.Printf("Độ sáng LED:     %s / 255\n", getLEDScale())
		return
	}

	// 2. Chế độ kiểm tra/sửa âm thanh
	if *audioFlag {
		// Nạp cấu hình để dùng đúng audio.dac_volume; thiếu file thì dùng mặc định.
		cfg, err := LoadConfig(configPath)
		if err != nil {
			log.Printf("[AUDIO] Không đọc được cấu hình (%v), dùng mức mặc định", err)
			cfg = DefaultConfig()
		}
		if *dacFlag > 0 {
			// Cho phép thử nhanh một mức khác mà không phải sửa config.json.
			cfg.Audio.DACVolume = *dacFlag
			fmt.Printf("Ghi đè DAC volume: %d\n", *dacFlag)
		}
		fmt.Println("=== KIỂM TRA ÂM THANH (TRIMUI BRICK PRO) ===")
		dumpAudioState(cfg)
		fmt.Println("--- đang sửa ---")
		repairAudio(cfg)
		dac, gain := audioSettings(cfg)
		fmt.Printf("Sau khi sửa:       sink=%q âm lượng=%d%% mute=%v gain=%d/7 "+
			"digital=%s DAC=%s (đặt %d) PA=%s\n",
			defaultSink(), sinkVolume("@DEFAULT_SINK@"), sinkMuted("@DEFAULT_SINK@"),
			gain, mixerValue("digital volume"), mixerValue("DAC volume"), dac,
			speakerMuteState())
		if *toneFlag {
			if !playTestTone() {
				fmt.Println("Không phát được âm thử (thiếu paplay/aplay?)")
			}
		}
		return
	}

	// 3. Chế độ Probe dò tìm thiết bị
	if *probeFlag {
		// Đảm bảo thư mục cha tồn tại
		_ = os.MkdirAll(filepath.Dir(configPath), 0755)
		RunProbeMode(configPath)
		return
	}

	// 3. Khởi động Daemon chạy nền
	log.Println("=====================================================")
	log.Println("      TRIMUI BRICK PRO - FN SWITCH DAEMON v1.0       ")
	log.Println("=====================================================")
	log.Printf("[INIT] Đang nạp cấu hình từ: %s", configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("[FATAL] Không thể đọc cấu hình: %v", err)
	}

	hw := NewHardwareManager(cfg)

	// Tạo bàn phím ảo sớm để RetroArch kịp nhận diện trước khi bấm nút.
	initKeyInjector()

	// Bắt tín hiệu dừng SIGINT, SIGTERM để khôi phục cấu hình an toàn khi thoát
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("[SHUTDOWN] Nhận tín hiệu dừng. Khôi phục cấu hình gốc và thoát...")
		hw.ApplySwitchState(false, cfg)
		if injector != nil {
			_ = injector.Close()
		}
		os.Exit(0)
	}()

	// Bắt tín hiệu SIGHUP để nạp lại cấu hình trực tiếp từ Brick Pro FN Config
	hupChan := make(chan os.Signal, 1)
	signal.Notify(hupChan, syscall.SIGHUP)
	go func() {
		for range hupChan {
			log.Println("[RELOAD] Nhận tín hiệu SIGHUP. Đang nạp lại file cấu hình...")
			if newCfg, err := LoadConfig(configPath); err == nil {
				*cfg = *newCfg
				log.Println("[RELOAD] Cập nhật cấu hình thành công!")
			} else {
				log.Printf("[RELOAD ERROR] Không thể đọc lại cấu hình: %v", err)
			}
		}
	}()

	// Bắt đầu lắng nghe sự kiện
	if err := ListenEvents(cfg, hw); err != nil {
		log.Fatalf("[FATAL] %v", err)
	}
}
