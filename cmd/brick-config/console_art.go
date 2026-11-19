package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// ConsoleState trạng thái các nút bấm trên TrimUI Brick Pro.
//
// Các trường Lit là đèn LED dùng riêng cho bản vẽ tester. Chúng khác với trạng
// thái nút thật ở chỗ một số phím được đấu chung trên máy: X thắp đèn L1, D-pad
// lên thắp L2, D-pad xuống thắp R2 và START cũng thắp L2. Tách riêng để bấm X
// không bị coi là bấm L1 khi điều hướng menu.
type ConsoleState struct {
	Up, Down, Left, Right bool
	A, B, X, Y            bool
	L1, R1, L2, R2        bool
	Select, Start         bool
	F1, F2                bool
	Home                  bool
	FNSwitch              bool
	StickLX, StickLY      float64
	StickRX, StickRY      float64

	// Đèn quanh nút vai và nút phụ (hiệu ứng hiển thị, không phải trạng thái phím).
	L1Lit, L2Lit, R1Lit, R2Lit bool
	F1Lit, F2Lit               bool
}

// drawLedHalo vẽ vòng LED quanh một nút. Khi lit=true vòng sáng màu xanh ngọc
// và có bốn chấm trắng ở góc; khi tắt chỉ còn viền mờ để vẫn thấy vị trí nút.
func drawLedHalo(dst *ebiten.Image, x, y, w, h int, lit bool, th Theme) {
	const pad = 2
	hx, hy := x-pad, y-pad
	hw, hh := w+2*pad, h+2*pad
	if lit {
		frame(dst, hx, hy, hw, hh, th.Accent)
		corner := color.RGBA{0xe6, 0xff, 0xf8, 0xff}
		rect(dst, hx, hy, 2, 2, corner)
		rect(dst, hx+hw-2, hy, 2, 2, corner)
		rect(dst, hx, hy+hh-2, 2, 2, corner)
		rect(dst, hx+hw-2, hy+hh-2, 2, 2, corner)
	} else {
		frame(dst, hx, hy, hw, hh, color.RGBA{0x33, 0x3a, 0x56, 0xff})
	}
}

// DrawConsoleArt vẽ chi tiết máy TrimUI Brick Pro gồm cả Mặt Đỉnh (Nút vai L1/L2/R1/R2) và Mặt Trước
func DrawConsoleArt(dst *ebiten.Image, startX, startY int, state ConsoleState, th Theme) {
	btnOff := color.RGBA{0x2e, 0x34, 0x4a, 0xff}
	btnBorder := color.RGBA{0x44, 0x4e, 0x70, 0xff}
	btnLit := th.Accent2   // Sáng rực màu Amber (#FFB000)
	accentLit := th.Accent // Xanh ngọc phát sáng (#4FE3C1)
	caseBg := color.RGBA{0x20, 0x24, 0x36, 0xff}
	caseBorder := color.RGBA{0x3c, 0x43, 0x62, 0xff}

	// =========================================================================
	// 1. MẶT ĐỈNH / NÚT VAI (SHOULDER VIEW: L1 ngoài, L2 trong, LED, R2 trong, R1 ngoài)
	// =========================================================================
	shX := startX
	shY := startY
	shW := 114
	shH := 20

	// Nền mặt đỉnh máy, cao hơn để vừa vòng LED quanh bốn nút vai.
	panel(dst, shX, shY, shW, shH, color.RGBA{0x17, 0x1a, 0x28, 0xff})
	frame(dst, shX, shY, shW, shH, caseBorder)

	// --- Cụm vai Trái: L1 (NGOÀI CÙNG), L2 (BÊN TRONG) ---
	l1Col, l2Col := btnOff, btnOff
	l1TextCol, l2TextCol := th.TextDim, th.TextDim
	if state.L1Lit {
		l1Col = btnLit
		l1TextCol = color.RGBA{0x15, 0x0a, 0x00, 0xff}
	}
	if state.L2Lit {
		l2Col = btnLit
		l2TextCol = color.RGBA{0x15, 0x0a, 0x00, 0xff}
	}

	// Phím L1 (Bumper ngoài cùng bên trái)
	panel(dst, shX+3, shY+4, 19, 11, l1Col)
	if state.L1 {
		frame(dst, shX+3, shY+4, 19, 11, color.RGBA{0xff, 0xff, 0xff, 0xff})
	}
	drawLedHalo(dst, shX+3, shY+4, 19, 11, state.L1Lit, th)
	drawText(dst, "L1", SmallFace, shX+8, shY+6, l1TextCol)

	// Phím L2 (Trigger phía trong)
	panel(dst, shX+25, shY+5, 18, 10, l2Col)
	if state.L2 {
		frame(dst, shX+25, shY+5, 18, 10, color.RGBA{0xff, 0xff, 0xff, 0xff})
	}
	drawLedHalo(dst, shX+25, shY+5, 18, 10, state.L2Lit, th)
	drawText(dst, "L2", SmallFace, shX+30, shY+6, l2TextCol)

	// --- Dải LED Đỉnh máy (Top LED Bar ở giữa) ---
	topLedCol := color.RGBA{0x4a, 0x51, 0x70, 0xff}
	if !state.FNSwitch || state.F1Lit || state.F2Lit || state.L1Lit || state.L2Lit || state.R1Lit || state.R2Lit {
		topLedCol = accentLit
	}
	rect(dst, shX+47, shY+7, 20, 4, topLedCol)
	frame(dst, shX+46, shY+6, 22, 6, btnBorder)

	// --- Cụm vai Phải: R2 (BÊN TRONG), R1 (NGOÀI CÙNG) ---
	r2Col, r1Col := btnOff, btnOff
	r2TextCol, r1TextCol := th.TextDim, th.TextDim
	if state.R2Lit {
		r2Col = btnLit
		r2TextCol = color.RGBA{0x15, 0x0a, 0x00, 0xff}
	}
	if state.R1Lit {
		r1Col = btnLit
		r1TextCol = color.RGBA{0x15, 0x0a, 0x00, 0xff}
	}

	// Phím R2 (Trigger phía trong)
	panel(dst, shX+72, shY+5, 18, 10, r2Col)
	if state.R2 {
		frame(dst, shX+72, shY+5, 18, 10, color.RGBA{0xff, 0xff, 0xff, 0xff})
	}
	drawLedHalo(dst, shX+72, shY+5, 18, 10, state.R2Lit, th)
	drawText(dst, "R2", SmallFace, shX+77, shY+6, r2TextCol)

	// Phím R1 (Bumper ngoài cùng bên phải)
	panel(dst, shX+93, shY+4, 19, 11, r1Col)
	if state.R1 {
		frame(dst, shX+93, shY+4, 19, 11, color.RGBA{0xff, 0xff, 0xff, 0xff})
	}
	drawLedHalo(dst, shX+93, shY+4, 19, 11, state.R1Lit, th)
	drawText(dst, "R1", SmallFace, shX+98, shY+6, r1TextCol)

	// =========================================================================
	// 2. MẶT TRƯỚC MÁY (FRONT VIEW: TRIMUI BRICK PRO)
	// =========================================================================
	x := startX
	y := startY + 22
	w := 114
	h := 134

	// Gờ nút vai nhô lên từ phía trước (L1 ngoài, L2 trong, R2 trong, R1 ngoài)
	rect(dst, x+3, y-2, 14, 2, l1Col)
	rect(dst, x+20, y-2, 14, 2, l2Col)
	rect(dst, x+w-34, y-2, 14, 2, r2Col)
	rect(dst, x+w-17, y-2, 14, 2, r1Col)

	// Thân máy mặt trước
	panel(dst, x, y, w, h, caseBg)
	frame(dst, x, y, w, h, caseBorder)

	// Công tắc gạt FN Slider bên cạnh phải thân máy
	fnSlotY := y + 36
	fnSlotH := 24
	rect(dst, x+w, fnSlotY, 4, fnSlotH, color.RGBA{0x15, 0x18, 0x24, 0xff})
	frame(dst, x+w, fnSlotY, 4, fnSlotH, caseBorder)

	knobY := fnSlotY + 12
	knobCol := color.RGBA{0x7a, 0x82, 0xa3, 0xff}
	if state.FNSwitch {
		knobY = fnSlotY + 1 // Gạt lên vị trí ON
		knobCol = th.Accent
	}
	panel(dst, x+w, knobY, 4, 11, knobCol)

	// Phím Home cạnh trái thân máy
	homeSideCol := btnOff
	if state.Home {
		homeSideCol = btnLit
	}
	rect(dst, x-3, y+40, 3, 14, homeSideCol)
	frame(dst, x-3, y+40, 3, 14, caseBorder)

	// --- MÀN HÌNH CHÍNH (4:3 IPS Screen) ---
	scrX, scrY, scrW, scrH := x+10, y+8, w-20, 52
	panel(dst, scrX, scrY, scrW, scrH, color.RGBA{0x0e, 0x10, 0x18, 0xff})
	frame(dst, scrX, scrY, scrW, scrH, color.RGBA{0x2b, 0x31, 0x48, 0xff})
	rect(dst, scrX+2, scrY+2, scrW-4, scrH-4, color.RGBA{0x14, 0x19, 0x28, 0xff})

	// Logo TrimUI nhỏ dưới màn hình
	drawText(dst, "TRIMUI", SmallFace, x+w/2-14, scrY+scrH+1, color.RGBA{0x55, 0x5e, 0x80, 0xff})

	// =========================================================================
	// 3. CỤM NÚT PHỤ F1 & F2 (Có đèn LED viền riêng trong tester)
	// =========================================================================
	f1Col, f2Col := btnOff, btnOff
	f1TextCol, f2TextCol := color.RGBA{0x80, 0x8a, 0xaa, 0xff}, color.RGBA{0x80, 0x8a, 0xaa, 0xff}
	if state.F1Lit {
		f1Col = btnLit
		f1TextCol = color.RGBA{0x20, 0x10, 0x00, 0xff}
	}
	if state.F2Lit {
		f2Col = btnLit
		f2TextCol = color.RGBA{0x20, 0x10, 0x00, 0xff}
	}

	// Nút F1
	f1X, f1Y := x+w/2-19, y+68
	panel(dst, f1X, f1Y, 16, 7, f1Col)
	if state.F1 {
		frame(dst, f1X, f1Y, 16, 7, color.RGBA{0xff, 0xff, 0xff, 0xff})
	}
	drawLedHalo(dst, f1X, f1Y, 16, 7, state.F1Lit, th)
	drawText(dst, "F1", SmallFace, f1X+4, f1Y+1, f1TextCol)

	// Nút F2
	f2X, f2Y := x+w/2+3, y+68
	panel(dst, f2X, f2Y, 16, 7, f2Col)
	if state.F2 {
		frame(dst, f2X, f2Y, 16, 7, color.RGBA{0xff, 0xff, 0xff, 0xff})
	}
	drawLedHalo(dst, f2X, f2Y, 16, 7, state.F2Lit, th)
	drawText(dst, "F2", SmallFace, f2X+4, f2Y+1, f2TextCol)

	// =========================================================================
	// 4. CỤM D-PAD (Bên trái)
	// =========================================================================
	dpadX, dpadY := x+12, y+74
	dpadBg := color.RGBA{0x2c, 0x31, 0x48, 0xff}
	rect(dst, dpadX+7, dpadY, 7, 21, dpadBg)
	rect(dst, dpadX, dpadY+7, 21, 7, dpadBg)

	if state.Up {
		rect(dst, dpadX+7, dpadY, 7, 7, btnLit)
	}
	if state.Down {
		rect(dst, dpadX+7, dpadY+14, 7, 7, btnLit)
	}
	if state.Left {
		rect(dst, dpadX, dpadY+7, 7, 7, btnLit)
	}
	if state.Right {
		rect(dst, dpadX+14, dpadY+7, 7, 7, btnLit)
	}
	frame(dst, dpadX+6, dpadY-1, 9, 23, btnBorder)
	frame(dst, dpadX-1, dpadY+6, 23, 9, btnBorder)

	// =========================================================================
	// 5. CỤM NÚT ABXY (Bên phải - Diamond layout có chữ)
	// =========================================================================
	abxyX, abxyY := x+w-35, y+73
	bSize := 8

	drawABXYBtn := func(bx, by int, pressed bool, label string) {
		col := btnOff
		tCol := th.TextDim
		if pressed {
			col = btnLit
			tCol = color.RGBA{0x20, 0x10, 0x00, 0xff}
		}
		panel(dst, bx, by, bSize, bSize, col)
		if pressed {
			frame(dst, bx-1, by-1, bSize+2, bSize+2, color.RGBA{0xff, 0xff, 0xff, 0xff})
		} else {
			frame(dst, bx, by, bSize, bSize, btnBorder)
		}
		drawText(dst, label, SmallFace, bx+2, by+1, tCol)
	}

	// X (trên), Y (trái), B (dưới), A (phải)
	drawABXYBtn(abxyX+8, abxyY, state.X, "X")
	drawABXYBtn(abxyX, abxyY+8, state.Y, "Y")
	drawABXYBtn(abxyX+8, abxyY+16, state.B, "B")
	drawABXYBtn(abxyX+16, abxyY+8, state.A, "A")

	// =========================================================================
	// 6. PHÍM CHỨC NĂNG: SELECT, START
	// Nút Home vật lý nằm ở cạnh trái thân máy (đã vẽ phía trên), không vẽ lặp
	// trên mặt trước để tránh trùng hiển thị trong tester.
	// =========================================================================
	selCol := btnOff
	if state.Select {
		selCol = btnLit
	}
	staCol := btnOff
	if state.Start {
		staCol = btnLit
	}

	// Select
	panel(dst, x+w/2-16, y+100, 14, 5, selCol)
	frame(dst, x+w/2-16, y+100, 14, 5, btnBorder)

	// Start
	panel(dst, x+w/2+2, y+100, 14, 5, staCol)
	frame(dst, x+w/2+2, y+100, 14, 5, btnBorder)

	// =========================================================================
	// 7. CẦN ANALOG TRÁI & PHẢI (Có vòng LED Halo)
	// =========================================================================
	stickR := 8
	stickLY := y + 117
	stickLX := x + 25
	stickRX := x + w - 25

	drawStick := func(cx, cy int, lx, ly float64) {
		// Vòng LED Halo
		haloCol := color.RGBA{0x44, 0x4f, 0x73, 0xff}
		if !state.FNSwitch {
			haloCol = accentLit
		}
		frame(dst, cx-stickR-1, cy-stickR-1, (stickR+1)*2, (stickR+1)*2, haloCol)
		panel(dst, cx-stickR, cy-stickR, stickR*2, stickR*2, color.RGBA{0x13, 0x16, 0x22, 0xff})

		// Núm joystick gạt
		jx := cx + int(lx*3)
		jy := cy + int(ly*3)
		panel(dst, jx-4, jy-4, 8, 8, color.RGBA{0x65, 0x6e, 0x91, 0xff})
		frame(dst, jx-4, jy-4, 8, 8, btnBorder)
	}

	drawStick(stickLX, stickLY, state.StickLX, state.StickLY)
	drawStick(stickRX, stickLY, state.StickRX, state.StickRY)
}
