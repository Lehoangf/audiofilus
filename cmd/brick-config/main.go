package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

type Game struct {
	cfg           *AppConfig
	ui            *UI
	input         ControllerInput
	tracker       InputTracker
	cState        ConsoleState
	hwInfo        HardwareInfo
	hwTick        int
	exitHoldTicks int // Đếm số frame giữ tổ hợp SELECT+START để thoát
	shouldQuit    bool
}

func (g *Game) Update() error {
	if g.shouldQuit {
		return ebiten.Termination
	}

	// 1. Cập nhật phần cứng mỗi 30 frame (~0.5 giây)
	g.hwTick++
	if g.hwTick >= 30 {
		g.hwTick = 0
		g.hwInfo = ReadHardwareInfo()
		g.cState.FNSwitch = g.hwInfo.FNSwitchOn
	}

	// 2. Đọc input
	g.input.Update(&g.cState)
	g.cState.FNSwitch = g.hwInfo.FNSwitchOn
	up, down, left, right, a, b, start, selectBtn, l1Just, r1Just := g.tracker.CheckNav(g.cState)

	// =========================================================================
	// 3. TỔ HỢP THOÁT TOÀN ỨNG DỤNG: GIỮ SELECT + START TRONG 1 GIÂY
	// =========================================================================
	if g.cState.Select && g.cState.Start {
		g.exitHoldTicks++
		g.ui.ExitHoldProgress = float64(g.exitHoldTicks) / 60.0
		if g.exitHoldTicks >= 60 {
			g.shouldQuit = true
			return nil
		}
		// Đang giữ tổ hợp thoát thì không kích hoạt chức năng riêng lẻ
		return nil
	} else {
		g.exitHoldTicks = 0
		g.ui.ExitHoldProgress = 0
	}

	// =========================================================================
	// 4. XỬ LÝ HỘP THOẠI XÁC NHẬN THOÁT
	// =========================================================================
	if g.ui.ConfirmExit {
		if a {
			// Bấm A: Xác nhận thoát ứng dụng
			g.shouldQuit = true
			return nil
		}
		if b || up || down || left || right {
			// Bấm B hoặc D-pad: Hủy thoát và tiếp tục sử dụng
			g.ui.ConfirmExit = false
			return nil
		}
		return nil
	}

	// =========================================================================
	// 5. CHUYỂN TAB (WORKFLOW TỐI ƯU CHO TESTER):
	// - Ở Tab 4 (TEST PHẦN CỨNG): CHỈ PHÍM SELECT MỚI CHUYỂN TAB! L1 và R1 tuyệt đối không chuyển tab để test nút tự do!
	// - Ở Tab 0, 1, 2: Dùng SELECT hoặc L1 / R1 để chuyển tab linh hoạt.
	// =========================================================================
	if g.ui.CurrentTab == 3 {
		if selectBtn && !g.cState.Start {
			g.ui.CurrentTab = (g.ui.CurrentTab + 1) % len(g.ui.Tabs)
			g.ui.SelectedIdx = 0
			g.ui.ConfirmExit = false
		}
	} else {
		if selectBtn && !g.cState.Start {
			g.ui.CurrentTab = (g.ui.CurrentTab + 1) % len(g.ui.Tabs)
			g.ui.SelectedIdx = 0
			g.ui.ConfirmExit = false
		} else if r1Just && !g.cState.Start {
			g.ui.CurrentTab = (g.ui.CurrentTab + 1) % len(g.ui.Tabs)
			g.ui.SelectedIdx = 0
			g.ui.ConfirmExit = false
		} else if l1Just && !g.cState.Start {
			g.ui.CurrentTab = (g.ui.CurrentTab - 1 + len(g.ui.Tabs)) % len(g.ui.Tabs)
			g.ui.SelectedIdx = 0
			g.ui.ConfirmExit = false
		}
	}

	// 6. Xử lý phím B:
	// - Ở Tab 4 (TEST PHẦN CỨNG): KHÓA THOÁT HOÀN TOÀN để test nút B thoải mái!
	// - Ở Tab 0, 1, 2: Mở hộp thoại hỏi xác nhận có muốn thoát không.
	if b {
		if g.ui.CurrentTab == 3 {
			// Tab 4: Tự do test phím B, không thoát
		} else {
			g.ui.ConfirmExit = true
			return nil
		}
	}

	// 7. Lưu cấu hình khi bấm START
	if start {
		g.ui.ApplyToConfig(g.cfg)
		if err := SaveConfig(g.cfg); err != nil {
			g.ui.ShowToast("Lỗi lưu file cấu hình!")
		} else {
			g.ui.ShowToast("✓ ĐÃ LƯU & ÁP DỤNG THÀNH CÔNG!")
		}
	}

	// 6. Điều hướng trong danh sách Menu của Tab hiện tại
	items := g.ui.Items[g.ui.CurrentTab]
	nItems := len(items)

	if nItems > 0 {
		if up {
			g.ui.SelectedIdx--
			if g.ui.SelectedIdx < 0 {
				g.ui.SelectedIdx = nItems - 1
			}
		}
		if down {
			g.ui.SelectedIdx++
			if g.ui.SelectedIdx >= nItems {
				g.ui.SelectedIdx = 0
			}
		}

		curr := &items[g.ui.SelectedIdx]

		// Thay đổi giá trị (Left / Right)
		if left {
			switch curr.Type {
			case TypeToggle:
				curr.Value = 1 - curr.Value
			case TypeSlider:
				curr.Value -= curr.Step
				if curr.Value < curr.Min {
					curr.Value = curr.Min
				}
			case TypeChoice:
				curr.ChoiceIdx--
				if curr.ChoiceIdx < 0 {
					curr.ChoiceIdx = len(curr.Choices) - 1
				}
			}
		}

		if right {
			switch curr.Type {
			case TypeToggle:
				curr.Value = 1 - curr.Value
			case TypeSlider:
				curr.Value += curr.Step
				if curr.Value > curr.Max {
					curr.Value = curr.Max
				}
			case TypeChoice:
				curr.ChoiceIdx++
				if curr.ChoiceIdx >= len(curr.Choices) {
					curr.ChoiceIdx = 0
				}
			}
		}

		// Nút A (Kích hoạt hoặc Đổi trạng thái)
		if a {
			switch curr.Type {
			case TypeToggle:
				curr.Value = 1 - curr.Value
			case TypeChoice:
				curr.ChoiceIdx = (curr.ChoiceIdx + 1) % len(curr.Choices)
			case TypeAction:
				if curr.ID == "led_test_btn" {
					// Thử nghiệm live LED
					g.ui.ApplyToConfig(g.cfg)
					LivePreviewLEDs(g.cfg.Actions.LEDs.ScaleOn, g.cfg.Actions.LEDs.ScaleLR, g.cfg.Actions.LEDs.ScaleRear, g.cfg.Actions.LEDs.ScaleF1F2, !g.cfg.Actions.LEDs.BreathOff)
					g.ui.ShowToast("Đang phát hiệu ứng thử nghiệm LED!")
				}
			}
		}
	}

	// 7. Cập nhật hiệu ứng UI
	g.ui.Update()
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.ui.Draw(screen, DefaultTheme, g.hwInfo, g.cState)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 320, 240
}

func main() {
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		_ = os.Setenv("XDG_RUNTIME_DIR", "/run")
	}
	if os.Getenv("PULSE_SERVER") == "" {
		_ = os.Setenv("PULSE_SERVER", "unix:/var/run/pulse/native")
	}

	// Khởi chạy bộ lắng nghe sự kiện evdev trên Linux cho F1, F2, Home, FN Switch
	StartEvdevListener()

	// Khởi tạo font chữ pixel art tiếng Việt
	if err := initFonts(); err != nil {
		log.Printf("Cảnh báo nạp font: %v", err)
	}

	// Nạp file cấu hình
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Printf("Lỗi nạp config: %v, khởi tạo mặc định\n", err)
		cfg = DefaultAppConfig()
	}

	game := &Game{
		cfg:    cfg,
		ui:     NewUI(cfg),
		hwInfo: ReadHardwareInfo(),
	}

	// Thiết lập cửa sổ Ebitengine
	ebiten.SetWindowSize(960, 720) // Phóng to 3x chuẩn pixel khi chạy trên máy tính
	ebiten.SetWindowTitle("Brick Pro FN Config")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(game); err != nil && err != ebiten.Termination {
		fmt.Fprintf(os.Stderr, "Lỗi chạy game: %v\n", err)
		os.Exit(1)
	}
}
