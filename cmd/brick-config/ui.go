package main

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

type ItemType int

const (
	TypeToggle ItemType = iota
	TypeSlider
	TypeChoice
	TypeAction
)

type MenuItem struct {
	ID        string
	Label     string
	Type      ItemType
	Value     int      // Cho slider hoặc toggle (0/1)
	Min       int      // Cho slider
	Max       int      // Cho slider
	Step      int      // Cho slider
	Choices   []string // Cho choice
	ChoiceIdx int      // Index được chọn
	ActionFn  func()   // Cho action
}

type UI struct {
	Tabs             []string
	CurrentTab       int
	Items            [][]MenuItem
	SelectedIdx      int
	ToastMsg         string
	ToastTicks       int
	Tick             int
	ConfirmExit      bool    // Đang mở hộp thoại xác nhận thoát
	ExitHoldProgress float64 // Tiến trình giữ SELECT+START để thoát (0.0 -> 1.0)
}

func NewUI(cfg *AppConfig) *UI {
	ui := &UI{
		Tabs:       []string{"GẠT FN", "NÚT PHỤ", "ĐÈN LED", "PHẦN CỨNG"},
		CurrentTab: 0,
		Items:      make([][]MenuItem, 4),
	}
	ui.BuildItems(cfg)
	return ui
}

func (u *UI) BuildItems(cfg *AppConfig) {
	// --- TAB 0: CẤU HÌNH GẠT FN ---
	u.Items[0] = []MenuItem{
		{
			ID:        "fn_cpu_gov",
			Label:     "CPU khi gạt ON",
			Type:      TypeChoice,
			Choices:   []string{"powersave (480M)", "schedutil (Auto)", "performance (1.8G)"},
			ChoiceIdx: govToIdx(cfg.Actions.CPU.GovernorOn),
		},
		{
			ID:    "fn_bright",
			Label: "Độ sáng khi ON",
			Type:  TypeSlider,
			Value: cfg.Actions.Brightness.PercentOn,
			Min:   0,
			Max:   100,
			Step:  5,
		},
		{
			ID:    "fn_pause_media",
			Label: "Tạm dừng phát nhạc khi ON",
			Type:  TypeToggle,
			Value: boolToInt(cfg.Actions.Volume.PauseMedia),
		},
		{
			ID:    "fn_vol_mute",
			Label: "Mute tiếng khi ON",
			Type:  TypeToggle,
			Value: boolToInt(cfg.Actions.Volume.MuteOn),
		},
		{
			ID:    "fn_vol_pct",
			Label: "Âm lượng khi ON",
			Type:  TypeSlider,
			Value: cfg.Actions.Volume.PercentOn,
			Min:   0,
			Max:   100,
			Step:  5,
		},
		{
			ID:    "fn_led_off",
			Label: "Tắt toàn bộ LED khi ON",
			Type:  TypeToggle,
			Value: boolToInt(cfg.Actions.LEDs.TurnOffAll),
		},
		{
			ID:    "fn_led_breath",
			Label: "Tắt hiệu ứng thở khi ON",
			Type:  TypeToggle,
			Value: boolToInt(cfg.Actions.LEDs.BreathOff),
		},
	}

	// --- TAB 1: CẤU HÌNH NÚT PHỤ ---
	u.Items[1] = []MenuItem{
		{
			ID:        "btn_f1",
			Label:     "Hành động nút F1",
			Type:      TypeChoice,
			Choices:   []string{"Tắt tiếng (Mute)", "Chụp ảnh màn hình", "Giảm độ sáng", "Tua nhanh RetroArch", "Không gán"},
			ChoiceIdx: actionToIdx(cfg.Controls.F1Action, 0),
		},
		{
			ID:        "btn_f2",
			Label:     "Hành động nút F2",
			Type:      TypeChoice,
			Choices:   []string{"Bật/Tắt nhanh LED", "Tăng độ sáng", "Lưu nhanh RetroArch", "Không gán"},
			ChoiceIdx: actionToIdx(cfg.Controls.F2Action, 1),
		},
		{
			ID:        "btn_home",
			Label:     "Hành động phím Home",
			Type:      TypeChoice,
			Choices:   []string{"Mở Menu RetroArch", "Về EmulationStation", "Quick Menu", "Không gán"},
			ChoiceIdx: actionToIdx(cfg.Controls.HomeAction, 2),
		},
	}

	// --- TAB 2: ĐÈN LED (LED STUDIO) ---
	u.Items[2] = []MenuItem{
		{
			ID:    "led_top",
			Label: "Độ sáng dải LED đỉnh",
			Type:  TypeSlider,
			Value: cfg.Actions.LEDs.ScaleOn,
			Min:   0,
			Max:   255,
			Step:  15,
		},
		{
			ID:    "led_lr",
			Label: "Đèn viền Joystick L/R",
			Type:  TypeSlider,
			Value: cfg.Actions.LEDs.ScaleLR,
			Min:   0,
			Max:   40,
			Step:  2,
		},
		{
			ID:    "led_rear",
			Label: "Đèn nút vai L2/R2",
			Type:  TypeSlider,
			Value: cfg.Actions.LEDs.ScaleRear,
			Min:   0,
			Max:   40,
			Step:  2,
		},
		{
			ID:    "led_f1f2",
			Label: "Đèn nút F1 / F2",
			Type:  TypeSlider,
			Value: cfg.Actions.LEDs.ScaleF1F2,
			Min:   0,
			Max:   40,
			Step:  2,
		},
		{
			ID:    "led_test_btn",
			Label: "► Thử nghiệm trực tiếp trên máy",
			Type:  TypeAction,
		},
	}

	// --- TAB 3: PHẦN CỨNG & TESTER ---
	u.Items[3] = []MenuItem{
		{
			ID:    "hw_diag",
			Label: "Trạng thái cảm biến & Nút bấm",
			Type:  TypeAction,
		},
	}
}

func (u *UI) ApplyToConfig(cfg *AppConfig) {
	// Đọc từ Items tab 0
	for _, it := range u.Items[0] {
		switch it.ID {
		case "fn_cpu_gov":
			switch it.ChoiceIdx {
			case 0:
				cfg.Actions.CPU.GovernorOn = "powersave"
			case 1:
				cfg.Actions.CPU.GovernorOn = "schedutil"
			case 2:
				cfg.Actions.CPU.GovernorOn = "performance"
			}
		case "fn_bright":
			cfg.Actions.Brightness.PercentOn = it.Value
		case "fn_pause_media":
			cfg.Actions.Volume.PauseMedia = (it.Value == 1)
		case "fn_vol_mute":
			cfg.Actions.Volume.MuteOn = (it.Value == 1)
		case "fn_vol_pct":
			cfg.Actions.Volume.PercentOn = it.Value
		case "fn_led_off":
			cfg.Actions.LEDs.TurnOffAll = (it.Value == 1)
		case "fn_led_breath":
			cfg.Actions.LEDs.BreathOff = (it.Value == 1)
		}
	}

	// Đọc từ Items tab 1
	for _, it := range u.Items[1] {
		switch it.ID {
		case "btn_f1":
			cfg.Controls.F1Action = idxToAction(it.ChoiceIdx, 0)
		case "btn_f2":
			cfg.Controls.F2Action = idxToAction(it.ChoiceIdx, 1)
		case "btn_home":
			cfg.Controls.HomeAction = idxToAction(it.ChoiceIdx, 2)
		}
	}

	// Đọc từ Items tab 2
	for _, it := range u.Items[2] {
		switch it.ID {
		case "led_top":
			cfg.Actions.LEDs.ScaleOn = it.Value
		case "led_lr":
			cfg.Actions.LEDs.ScaleLR = it.Value
		case "led_rear":
			cfg.Actions.LEDs.ScaleRear = it.Value
		case "led_f1f2":
			cfg.Actions.LEDs.ScaleF1F2 = it.Value
		}
	}
}

func (u *UI) ShowToast(msg string) {
	u.ToastMsg = msg
	u.ToastTicks = 120 // ~2 giây ở 60fps
}

func (u *UI) Update() {
	u.Tick++
	if u.ToastTicks > 0 {
		u.ToastTicks--
	}
}

func (u *UI) Draw(dst *ebiten.Image, th Theme, hw HardwareInfo, cState ConsoleState) {
	// 1. Nền tổng thể
	dst.Fill(th.Bg)

	// 2. Header
	u.drawHeader(dst, th, hw)

	// 3. Tab Bar
	u.drawTabBar(dst, th)

	// 4. Nội dung Tab
	if u.CurrentTab == 3 {
		// Tab 3 là Gamepad & Hardware Tester đặc biệt
		u.drawHardwareTester(dst, th, hw, cState)
	} else {
		u.drawMenuList(dst, th)
	}

	// 5. Footer hướng dẫn
	u.drawFooter(dst, th)

	// 6. Toast thông báo nếu có
	if u.ToastTicks > 0 && u.ToastMsg != "" {
		u.drawToast(dst, th)
	}

	// 7. Thanh tiến trình giữ SELECT+START để thoát
	if u.ExitHoldProgress > 0 {
		u.drawExitHoldProgress(dst, th)
	}

	// 8. Hộp thoại xác nhận thoát
	if u.ConfirmExit {
		u.drawConfirmExitModal(dst, th)
	}
}

func (u *UI) drawHeader(dst *ebiten.Image, th Theme, hw HardwareInfo) {
	// Thanh header cao 22px
	rect(dst, 0, 0, 320, 22, th.PanelBg)
	rect(dst, 0, 22, 320, 1, th.PanelBorder)

	// Tên ứng dụng
	drawTextShadow(dst, "Brick Pro FN Config", TitleFace, 8, 4, th.Accent, th.PanelBorder)

	// Trạng thái công tắc FN
	fnText := "[ FN: OFF ]"
	fnCol := th.TextDim
	if hw.FNSwitchOn {
		fnText = "[ FN: ON ]"
		fnCol = th.Accent
	}
	drawText(dst, fnText, SmallFace, 195, 7, fnCol)

	// Pin
	batText := fmt.Sprintf("[%d%%]", hw.BatteryPercent)
	if hw.IsCharging {
		batText = fmt.Sprintf("[⚡%d%%]", hw.BatteryPercent)
	}
	drawText(dst, batText, SmallFace, 275, 7, th.Text)
}

func (u *UI) drawTabBar(dst *ebiten.Image, th Theme) {
	tabY := 26
	tabW := 74
	tabH := 18

	for i, tab := range u.Tabs {
		tx := 8 + i*(tabW+3)
		bg := th.PanelBg
		textCol := th.TextDim
		borderCol := th.PanelBorder

		if i == u.CurrentTab {
			bg = th.PanelFocus
			textCol = th.Accent
			borderCol = th.Accent
			marching(dst, tx, tabY, tabW, tabH, u.Tick, th.Accent)
		} else {
			frame(dst, tx, tabY, tabW, tabH, borderCol)
		}
		panel(dst, tx+1, tabY+1, tabW-2, tabH-2, bg)

		// Canh giữa text
		tw := textWidth(tab, SmallFace)
		drawText(dst, tab, SmallFace, tx+(tabW-tw)/2, tabY+5, textCol)
	}
}

func (u *UI) drawMenuList(dst *ebiten.Image, th Theme) {
	items := u.Items[u.CurrentTab]
	// Chiều cao gọn hơn để danh sách (tối đa 7 mục) không bị thanh hướng dẫn che.
	startY := 48
	itemH := 22
	itemW := 304
	itemX := 8

	for i, it := range items {
		iy := startY + i*(itemH+2)
		isSelected := (i == u.SelectedIdx)

		bg := th.PanelBg
		lblCol := th.Text
		if isSelected {
			bg = th.PanelFocus
			lblCol = th.Accent
		}

		panel(dst, itemX, iy, itemW, itemH, bg)
		if isSelected {
			marching(dst, itemX, iy, itemW, itemH, u.Tick, th.Accent)
		} else {
			frame(dst, itemX, iy, itemW, itemH, th.PanelBorder)
		}

		// Nhãn bên trái
		drawText(dst, it.Label, SmallFace, itemX+8, iy+6, lblCol)

		// Giá trị bên phải theo Type
		switch it.Type {
		case TypeToggle:
			swW, swH := 24, 12
			swX := itemX + itemW - swW - 8
			swY := iy + (itemH-swH)/2
			isOn := (it.Value == 1)

			tBg := th.SliderTrack
			knobCol := th.TextDim
			knobX := swX + 2
			if isOn {
				tBg = th.Accent
				knobCol = th.Text
				knobX = swX + swW - 10
			}
			panel(dst, swX, swY, swW, swH, tBg)
			rect(dst, knobX, swY+2, 8, swH-4, knobCol)

		case TypeSlider:
			slW, slH := 70, 8
			slX := itemX + itemW - slW - 38
			slY := iy + (itemH-slH)/2

			panel(dst, slX, slY, slW, slH, th.SliderTrack)
			fillW := int(float64(it.Value-it.Min) / float64(it.Max-it.Min) * float64(slW))
			if fillW > 0 {
				rect(dst, slX+1, slY+1, fillW, slH-2, th.Accent)
			}
			// Con trỏ
			rect(dst, slX+fillW-1, slY-1, 3, slH+2, th.Text)

			// Số phần trăm
			valStr := fmt.Sprintf("%d", it.Value)
			drawText(dst, valStr, SmallFace, itemX+itemW-28, iy+6, th.Text)

		case TypeChoice:
			choiceText := it.Choices[it.ChoiceIdx]
			cw := textWidth(choiceText, SmallFace)
			cx := itemX + itemW - cw - 16
			drawText(dst, "◄", SmallFace, cx-8, iy+6, th.TextDim)
			drawText(dst, choiceText, SmallFace, cx, iy+6, th.Accent2)
			drawText(dst, "►", SmallFace, cx+cw+3, iy+6, th.TextDim)

		case TypeAction:
			drawText(dst, "[ BẤM A ]", SmallFace, itemX+itemW-56, iy+6, th.Accent2)
		}
	}
}

func (u *UI) drawHardwareTester(dst *ebiten.Image, th Theme, hw HardwareInfo, cState ConsoleState) {
	// 1. Bên trái: Hình vẽ máy TrimUI Brick Pro gồm cả Mặt Đỉnh (L1/L2/R1/R2) và Mặt Trước
	DrawConsoleArt(dst, 10, 48, cState, th)

	// 2. Bên phải: Bảng thông số phần cứng & Bộ nhận diện phím bấm
	infoX := 130
	infoY := 48
	infoW := 182
	infoH := 162

	panel(dst, infoX, infoY, infoW, infoH, th.PanelBg)
	frame(dst, infoX, infoY, infoW, infoH, th.PanelBorder)

	// Tiêu đề
	drawText(dst, "THÔNG SỐ PHẦN CỨNG", TitleFace, infoX+8, infoY+6, th.Accent2)
	rect(dst, infoX+8, infoY+20, infoW-16, 1, th.PanelBorder)

	lines := []string{
		fmt.Sprintf("• CPU Temp : %.1f °C", hw.CPUTemp),
		fmt.Sprintf("• CPU Xung : %d MHz", hw.CPUFreqMHz),
		fmt.Sprintf("• Độ sáng  : %d %%", hw.Brightness),
		fmt.Sprintf("• Âm lượng : %d %%", hw.Volume),
		fmt.Sprintf("• Pin máy  : %d %% (%s)", hw.BatteryPercent, map[bool]string{true: "Sạc", false: "Dùng"}[hw.IsCharging]),
		fmt.Sprintf("• Gạt FN   : %s", map[bool]string{true: "BẬT (ON)", false: "TẮT (OFF)"}[cState.FNSwitch]),
	}

	for i, l := range lines {
		drawText(dst, l, SmallFace, infoX+8, infoY+24+i*13, th.Text)
	}

	// 3. Khu vực hiển thị phím bấm trực tiếp (Active Key Log)
	sepY := infoY + 102
	rect(dst, infoX+8, sepY, infoW-16, 1, th.PanelBorder)
	drawText(dst, "PHÍM ĐANG NHẬN:", SmallFace, infoX+8, sepY+4, th.Accent)

	// Gom các phím đang bấm
	var activeKeys []string
	if cState.F1 {
		activeKeys = append(activeKeys, "F1")
	}
	if cState.F2 {
		activeKeys = append(activeKeys, "F2")
	}
	if cState.Home {
		activeKeys = append(activeKeys, "HOME")
	}
	if cState.L1 {
		activeKeys = append(activeKeys, "L1")
	}
	if cState.L2 {
		activeKeys = append(activeKeys, "L2")
	}
	if cState.R1 {
		activeKeys = append(activeKeys, "R1")
	}
	if cState.R2 {
		activeKeys = append(activeKeys, "R2")
	}
	if cState.A {
		activeKeys = append(activeKeys, "A")
	}
	if cState.B {
		activeKeys = append(activeKeys, "B")
	}
	if cState.X {
		activeKeys = append(activeKeys, "X")
	}
	if cState.Y {
		activeKeys = append(activeKeys, "Y")
	}
	if cState.Up {
		activeKeys = append(activeKeys, "LÊN")
	}
	if cState.Down {
		activeKeys = append(activeKeys, "XUỐNG")
	}
	if cState.Left {
		activeKeys = append(activeKeys, "TRÁI")
	}
	if cState.Right {
		activeKeys = append(activeKeys, "PHẢI")
	}
	if cState.Select {
		activeKeys = append(activeKeys, "SELECT")
	}
	if cState.Start {
		activeKeys = append(activeKeys, "START")
	}
	if cState.FNSwitch {
		activeKeys = append(activeKeys, "FN")
	}

	badgeY := sepY + 17
	if len(activeKeys) == 0 {
		drawText(dst, "(Chưa bấm phím nào)", SmallFace, infoX+8, badgeY+3, th.TextDim)
		drawText(dst, "Test F1 F2 Home L R Start", SmallFace, infoX+8, badgeY+15, color.RGBA{0x55, 0x60, 0x85, 0xff})
	} else {
		// Vẽ các huy hiệu phím đang nhận
		curX := infoX + 8
		lineY := badgeY
		for _, k := range activeKeys {
			kw := textWidth(k, SmallFace) + 8
			if curX+kw > infoX+infoW-8 {
				curX = infoX + 8
				lineY += 14
			}
			panel(dst, curX, lineY, kw, 12, th.Accent2)
			drawText(dst, k, SmallFace, curX+4, lineY+1, color.RGBA{0x20, 0x10, 0x00, 0xff})
			curX += kw + 4
		}
	}
}

func (u *UI) drawFooter(dst *ebiten.Image, th Theme) {
	// Đặt cao hơn để không che mục cuối của danh sách và bảng phần cứng.
	footY := 216
	rect(dst, 0, footY, 320, 24, th.PanelBg)
	rect(dst, 0, footY, 320, 1, th.PanelBorder)

	var hints string
	if u.CurrentTab == 3 {
		hints = "Test phím/LED   SELECT: Đổi Tab   GIỮ SELECT+START: Thoát"
	} else {
		hints = "◄► Chỉnh  A Chọn  START Lưu  B Thoát  SELECT Tab"
	}
	drawText(dst, hints, SmallFace, 6, footY+7, th.TextDim)
}

func (u *UI) drawConfirmExitModal(dst *ebiten.Image, th Theme) {
	// Phủ nền mờ toàn màn hình
	rect(dst, 0, 0, 320, 240, color.RGBA{0x00, 0x00, 0x00, 0xb8})

	// Khung hộp thoại trung tâm
	mW, mH := 204, 72
	mX := (320 - mW) / 2
	mY := (240 - mH) / 2

	panel(dst, mX, mY, mW, mH, th.PanelBg)
	frame(dst, mX, mY, mW, mH, th.Accent)
	marching(dst, mX, mY, mW, mH, u.Tick, th.Accent)

	// Tiêu đề
	drawText(dst, "BẠN CÓ MUỐN THOÁT?", TitleFace, mX+20, mY+12, th.Text)
	rect(dst, mX+12, mY+30, mW-24, 1, th.PanelBorder)

	// Hai nút lựa chọn
	drawText(dst, "[ A ] THOÁT", SmallFace, mX+28, mY+44, th.Danger)
	drawText(dst, "[ B ] Ở LẠI", SmallFace, mX+120, mY+44, th.Success)
}

func (u *UI) drawExitHoldProgress(dst *ebiten.Image, th Theme) {
	pW := 226
	pH := 26
	pX := (320 - pW) / 2
	pY := 107

	panel(dst, pX, pY, pW, pH, color.RGBA{0x10, 0x12, 0x1e, 0xf6})
	frame(dst, pX, pY, pW, pH, th.Accent2)

	drawText(dst, "ĐANG THOÁT... (GIỮ SELECT+START)", SmallFace, pX+12, pY+5, th.Accent2)

	// Thanh đếm ngược tiến trình
	bW := pW - 20
	rect(dst, pX+10, pY+16, bW, 5, th.SliderTrack)
	fillW := int(u.ExitHoldProgress * float64(bW))
	if fillW > bW {
		fillW = bW
	}
	if fillW > 0 {
		rect(dst, pX+10, pY+16, fillW, 5, th.Danger)
	}
}

func (u *UI) drawToast(dst *ebiten.Image, th Theme) {
	tw := textWidth(u.ToastMsg, SmallFace) + 24
	thH := 20
	tx := (320 - tw) / 2
	ty := 194

	panel(dst, tx, ty, tw, thH, color.RGBA{0x00, 0x99, 0x4d, 0xff})
	frame(dst, tx, ty, tw, thH, th.Success)
	drawTextShadow(dst, u.ToastMsg, SmallFace, tx+12, ty+6, th.Text, color.RGBA{0x00, 0x33, 0x11, 0xff})
}

// Helpers
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func govToIdx(gov string) int {
	switch strings.ToLower(gov) {
	case "powersave":
		return 0
	case "schedutil":
		return 1
	case "performance":
		return 2
	default:
		return 0
	}
}

func actionToIdx(act string, kind int) int {
	switch kind {
	case 0: // F1
		switch act {
		case "mute":
			return 0
		case "screenshot":
			return 1
		case "brightness_down":
			return 2
		case "retroarch_ff":
			return 3
		default:
			return 4
		}
	case 1: // F2
		switch act {
		case "led_toggle":
			return 0
		case "brightness_up":
			return 1
		case "retroarch_save":
			return 2
		default:
			return 3
		}
	case 2: // Home
		switch act {
		case "retroarch_menu":
			return 0
		case "exit_es":
			return 1
		case "quick_menu":
			return 2
		default:
			return 3
		}
	}
	return 0
}

func idxToAction(idx int, kind int) string {
	switch kind {
	case 0: // F1
		m := []string{"mute", "screenshot", "brightness_down", "retroarch_ff", "none"}
		if idx >= 0 && idx < len(m) {
			return m[idx]
		}
	case 1: // F2
		m := []string{"led_toggle", "brightness_up", "retroarch_save", "none"}
		if idx >= 0 && idx < len(m) {
			return m[idx]
		}
	case 2: // Home
		m := []string{"retroarch_menu", "exit_es", "quick_menu", "none"}
		if idx >= 0 && idx < len(m) {
			return m[idx]
		}
	}
	return "none"
}
