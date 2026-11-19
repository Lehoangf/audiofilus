package ui

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Official rainbow trail colors for Nyan Cat
var (
	nyanRed    = color.RGBA{0xff, 0x00, 0x00, 0xff}
	nyanOrange = color.RGBA{0xff, 0x99, 0x00, 0xff}
	nyanYellow = color.RGBA{0xff, 0xff, 0x00, 0xff}
	nyanGreen  = color.RGBA{0x33, 0xff, 0x00, 0xff}
	nyanBlue   = color.RGBA{0x00, 0x99, 0xff, 0xff}
	nyanPurple = color.RGBA{0x66, 0x33, 0xff, 0xff}

	nyanToastBorder = color.RGBA{0xb8, 0x7b, 0x44, 0xff}
	nyanFrosting    = color.RGBA{0xff, 0x99, 0xcc, 0xff}
	nyanSprinkles   = color.RGBA{0xff, 0x33, 0x99, 0xff}
	nyanCatGray     = color.RGBA{0x99, 0x99, 0x99, 0xff}
	nyanCheekPink   = color.RGBA{0xff, 0x99, 0xaa, 0xff}
	nyanBlack       = color.RGBA{0x11, 0x11, 0x11, 0xff}
	nyanWhite       = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// drawNyanCatProgress renders a Nyan Cat progress bar with an oscillating 6-band rainbow trail.
func drawNyanCatProgress(dst *ebiten.Image, bx, y, bw int, progress float64, t float64) {
	if bw <= 0 {
		return
	}
	fill := int(float64(bw) * math.Max(0, math.Min(1, progress)))

	// Remaining progress track background
	if fill < bw {
		rect(dst, bx+fill, y+1, bw-fill, 3, color.RGBA{0x22, 0x22, 0x33, 0x66})
	}

	// 1. Oscillating rainbow trail (6 1px bands)
	rainbowColors := [6]color.Color{nyanRed, nyanOrange, nyanYellow, nyanGreen, nyanBlue, nyanPurple}
	waveEnd := fill
	if waveEnd > bw {
		waveEnd = bw
	}

	for col := 0; col < waveEnd; col++ {
		curX := bx + col
		wave := int(math.Sin(float64(col)*0.16-t*11.0) * 1.5)
		baseY := y - 1 + wave
		for b := 0; b < 6; b++ {
			rect(dst, curX, baseY+b, 1, 1, rainbowColors[b])
		}
	}

	// 2. Nyan Cat sprite as progress knob
	waveCat := int(math.Sin(float64(fill)*0.16-t*11.0) * 1.5)
	catX := bx + fill - 8
	catY := y - 5 + waveCat
	frame := int(t*8.0) % 2

	drawNyanCatSprite(dst, catX, catY, frame)
}

// drawNyanCatSprite renders the 16x11 Nyan Cat sprite with a 2-frame animation cycle.
func drawNyanCatSprite(dst *ebiten.Image, x, y, frame int) {
	// Tail (moves up/down based on frame)
	tailY := y + 4
	if frame == 1 {
		tailY = y + 3
	}
	rect(dst, x-2, tailY, 2, 2, nyanCatGray)
	rect(dst, x-3, tailY+1, 1, 1, nyanBlack)

	// Paws
	pawsY := y + 9
	if frame == 0 {
		rect(dst, x+1, pawsY, 2, 1, nyanCatGray)
		rect(dst, x+7, pawsY, 2, 1, nyanCatGray)
	} else {
		rect(dst, x+2, pawsY, 2, 1, nyanCatGray)
		rect(dst, x+6, pawsY, 2, 1, nyanCatGray)
	}

	// Pop-Tart body: pastry crust
	rect(dst, x, y+1, 10, 8, nyanToastBorder)
	// Pink frosting
	rect(dst, x+1, y+2, 8, 6, nyanFrosting)
	// Sprinkles
	rect(dst, x+2, y+3, 1, 1, nyanSprinkles)
	rect(dst, x+6, y+3, 1, 1, nyanSprinkles)
	rect(dst, x+4, y+5, 1, 1, nyanSprinkles)
	rect(dst, x+7, y+6, 1, 1, nyanSprinkles)
	rect(dst, x+3, y+6, 1, 1, nyanSprinkles)

	// Cat head
	headX := x + 7
	headY := y + 2
	rect(dst, headX, headY+1, 7, 6, nyanCatGray)

	// Ears
	rect(dst, headX+1, headY, 2, 1, nyanCatGray)
	rect(dst, headX+5, headY, 2, 1, nyanCatGray)

	// Eyes (black with white specular highlight)
	rect(dst, headX+2, headY+2, 1, 2, nyanBlack)
	rect(dst, headX+5, headY+2, 1, 2, nyanBlack)
	rect(dst, headX+2, headY+2, 1, 1, nyanWhite)
	rect(dst, headX+5, headY+2, 1, 1, nyanWhite)

	// Rosy cheeks
	rect(dst, headX+1, headY+4, 1, 1, nyanCheekPink)
	rect(dst, headX+6, headY+4, 1, 1, nyanCheekPink)

	// Nose
	rect(dst, headX+4, headY+4, 1, 1, nyanBlack)
}
