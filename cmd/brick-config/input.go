package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type ControllerInput struct {
	Pads []ebiten.GamepadID
}

func (in *ControllerInput) Update(cState *ConsoleState) {
	in.Pads = ebiten.AppendGamepadIDs(in.Pads[:0])

	// Reset state
	*cState = ConsoleState{}

	// 1. Đọc từ Bàn phím (Hỗ trợ thử nghiệm trên PC)
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		cState.Up = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		cState.Down = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		cState.Left = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		cState.Right = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeyZ) {
		cState.A = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyEscape) || ebiten.IsKeyPressed(ebiten.KeyX) {
		cState.B = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyC) {
		cState.X = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyV) {
		cState.Y = true
	}
	if ebiten.IsKeyPressed(ebiten.Key1) || ebiten.IsKeyPressed(ebiten.KeyQ) {
		cState.L1 = true
	}
	if ebiten.IsKeyPressed(ebiten.Key2) || ebiten.IsKeyPressed(ebiten.KeyE) {
		cState.R1 = true
	}
	if ebiten.IsKeyPressed(ebiten.Key3) {
		cState.L2 = true
	}
	if ebiten.IsKeyPressed(ebiten.Key4) {
		cState.R2 = true
	}
	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		cState.Start = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyTab) {
		cState.Select = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyF1) {
		cState.F1 = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyF2) {
		cState.F2 = true
	}
	if ebiten.IsKeyPressed(ebiten.KeyHome) {
		cState.Home = true
	}

	// 2. Đọc từ Tay cầm Gamepad (Ebitengine Standard Gamepad)
	for _, id := range in.Pads {
		if ebiten.IsStandardGamepadLayoutAvailable(id) {
			// D-Pad
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftTop) {
				cState.Up = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftBottom) {
				cState.Down = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftLeft) {
				cState.Left = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftRight) {
				cState.Right = true
			}

			// Nút ABXY
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightBottom) { // A
				cState.A = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightRight) { // B
				cState.B = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightLeft) { // X
				cState.X = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightTop) { // Y
				cState.Y = true
			}

			// Vai L/R (Hỗ trợ cả nút bấm và mức nhấn analog trigger)
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonFrontTopLeft) {
				cState.L1 = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonFrontTopRight) {
				cState.R1 = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonFrontBottomLeft) ||
				ebiten.StandardGamepadButtonValue(id, ebiten.StandardGamepadButtonFrontBottomLeft) > 0.25 {
				cState.L2 = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonFrontBottomRight) ||
				ebiten.StandardGamepadButtonValue(id, ebiten.StandardGamepadButtonFrontBottomRight) > 0.25 {
				cState.R2 = true
			}

			// Select, Start, Guide/Home
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonCenterLeft) {
				cState.Select = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonCenterRight) {
				cState.Start = true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonCenterCenter) {
				cState.Home = true
			}

			// Analog Sticks
			lx := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
			ly := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical)
			if math.Abs(lx) > 0.15 || math.Abs(ly) > 0.15 {
				cState.StickLX = lx
				cState.StickLY = ly
				if ly < -0.5 {
					cState.Up = true
				}
				if ly > 0.5 {
					cState.Down = true
				}
				if lx < -0.5 {
					cState.Left = true
				}
				if lx > 0.5 {
					cState.Right = true
				}
			}

			rx := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisRightStickHorizontal)
			ry := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisRightStickVertical)
			if math.Abs(rx) > 0.15 || math.Abs(ry) > 0.15 {
				cState.StickRX = rx
				cState.StickRY = ry
			}
		}

		// Kiểm tra thêm raw buttons của gamepad nếu SDL mapping có độ trễ
		btnCount := ebiten.GamepadButtonCount(id)
		for b := ebiten.GamepadButton(0); b < ebiten.GamepadButton(btnCount); b++ {
			if ebiten.IsGamepadButtonPressed(id, b) {
				switch b {
				case 4, 9:
					cState.L1 = true
				case 5, 10:
					cState.R1 = true
				case 6, 11:
					cState.L2 = true
				case 7, 12:
					cState.R2 = true
				case 15:
					cState.Home = true
				}
			}
		}
	}

	// 3. Đọc từ Evdev trực tiếp (Bắt chính xác các phím phần cứng TrimUI Brick Pro: F1, F2, Home, Vai L/R, FN)
	if globalEvdev.F1Pressed.Load() {
		cState.F1 = true
	}
	if globalEvdev.F2Pressed.Load() {
		cState.F2 = true
	}
	if globalEvdev.HomePressed.Load() {
		cState.Home = true
	}
	if globalEvdev.L1Pressed.Load() {
		cState.L1 = true
	}
	if globalEvdev.R1Pressed.Load() {
		cState.R1 = true
	}
	if globalEvdev.L2Pressed.Load() {
		cState.L2 = true
	}
	if globalEvdev.R2Pressed.Load() {
		cState.R2 = true
	}
	if globalEvdev.FNSwitch.Load() {
		cState.FNSwitch = true
	}

	// Ánh xạ đèn LED trong tab tester. Chỉ ảnh hưởng hiển thị: giữ nguyên các
	// trường L1/L2/R1/R2 dùng cho điều hướng để bấm X hay D-pad không bị hiểu nhầm.
	// Map theo phần cứng Brick Pro: X chia sẻ LED với L1, D-pad lên và START chia
	// sẻ LED với L2, D-pad xuống chia sẻ LED với R2.
	cState.L1Lit = cState.L1 || cState.X
	cState.R1Lit = cState.R1
	cState.L2Lit = cState.L2 || cState.Up || cState.Start
	cState.R2Lit = cState.R2 || cState.Down
	cState.F1Lit = cState.F1
	cState.F2Lit = cState.F2
}

// InputTracker kiểm tra hành động vừa được bấm
type InputTracker struct {
	prevUp, prevDown, prevLeft, prevRight bool
	prevA, prevB, prevX, prevY            bool
	prevSelect, prevStart                 bool
	prevL1, prevR1                        bool
	repeatCounter                         int
	repeatKey                             string
}

func (tr *InputTracker) CheckNav(curr ConsoleState) (up, down, left, right, a, b, start, selectBtn, l1Just, r1Just bool) {
	if curr.A && !tr.prevA {
		a = true
	}
	if curr.B && !tr.prevB {
		b = true
	}
	if curr.Start && !tr.prevStart {
		start = true
	}
	if curr.Select && !tr.prevSelect {
		selectBtn = true
	}
	if curr.L1 && !tr.prevL1 {
		l1Just = true
	}
	if curr.R1 && !tr.prevR1 {
		r1Just = true
	}

	handleDir := func(pressed bool, prev *bool, name string, out *bool) {
		if pressed {
			if !*prev {
				*out = true
				tr.repeatKey = name
				tr.repeatCounter = 20
			} else if tr.repeatKey == name {
				tr.repeatCounter--
				if tr.repeatCounter <= 0 {
					*out = true
					tr.repeatCounter = 6
				}
			}
		} else if *prev && tr.repeatKey == name {
			tr.repeatKey = ""
			tr.repeatCounter = 0
		}
		*prev = pressed
	}

	handleDir(curr.Up, &tr.prevUp, "up", &up)
	handleDir(curr.Down, &tr.prevDown, "down", &down)
	handleDir(curr.Left, &tr.prevLeft, "left", &left)
	handleDir(curr.Right, &tr.prevRight, "right", &right)

	tr.prevA = curr.A
	tr.prevB = curr.B
	tr.prevStart = curr.Start
	tr.prevSelect = curr.Select
	tr.prevL1 = curr.L1
	tr.prevR1 = curr.R1

	return
}
