package ui

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
)

// matrixCol represents a column of the Matrix digital rain.
type matrixCol struct {
	headY    float64   // Vertical position of the leading glyph (pixels)
	speed    float64   // Fall speed
	length   int       // Trail length in glyphs
	chars    [32]uint8 // Glyph indices in matrixGlyphs
	swapTick float64   // Timer for random glyph mutation
}

// matrixState holds state for all active digital rain columns.
type matrixState struct {
	cols  []matrixCol
	lastW int
	lastH int
	rng   *rand.Rand
}

// matrixGlyphs provides 4x5 bitmap patterns for fast rendering without font overhead.
var matrixGlyphs = [16][5]uint8{
	{0x6, 0x9, 0x9, 0x9, 0x6}, // 0
	{0x2, 0x6, 0x2, 0x2, 0x7}, // 1
	{0x6, 0x9, 0x2, 0x4, 0xf}, // 2
	{0xe, 0x1, 0x6, 0x1, 0xe}, // 3
	{0x9, 0x9, 0xf, 0x1, 0x1}, // 4
	{0xf, 0x8, 0xe, 0x1, 0xe}, // 5
	{0x6, 0x8, 0xe, 0x9, 0x6}, // 6
	{0xf, 0x1, 0x2, 0x4, 0x4}, // 7
	{0x6, 0x9, 0x6, 0x9, 0x6}, // 8
	{0x6, 0x9, 0x7, 0x1, 0x6}, // 9
	{0x6, 0x9, 0xf, 0x9, 0x9}, // A
	{0x4, 0xe, 0x4, 0x4, 0x4}, // +
	{0x9, 0x9, 0x6, 0x9, 0x9}, // X
	{0x0, 0xf, 0x0, 0xf, 0x0}, // =
	{0x6, 0x6, 0x6, 0x6, 0x6}, // |
	{0xf, 0x9, 0x9, 0x9, 0xf}, // []
}

// drawMatrix renders audio-reactive Matrix digital rain:
//   - Variant 0: Dense rain accelerated by bass and modulated by spectrum intensity.
//   - Variant 1: Matrix equalizer where column length and fall speed follow FFT bands.
func (a *App) drawMatrix(img *ebiten.Image) {
	lw, lh := img.Bounds().Dx(), img.Bounds().Dy()
	if lw < 8 || lh < 8 {
		return
	}
	level, bass, bands := a.visBands()
	pulse := a.beatPulse()

	const colW = 6 // 4px glyph + 2px margin
	const rowH = 7 // 5px glyph + 2px margin
	nCols := lw / colW
	if nCols < 1 {
		return
	}

	ms := a.vis.matrix
	if ms == nil || ms.lastW != lw || ms.lastH != lh || len(ms.cols) != nCols {
		ms = &matrixState{
			cols:  make([]matrixCol, nCols),
			lastW: lw,
			lastH: lh,
			rng:   rand.New(rand.NewSource(1337)),
		}
		for i := range ms.cols {
			ms.cols[i].headY = ms.rng.Float64() * float64(lh)
			ms.cols[i].speed = 1.0 + ms.rng.Float64()*2.2
			ms.cols[i].length = 8 + ms.rng.Intn(16)
			for c := range ms.cols[i].chars {
				ms.cols[i].chars[c] = uint8(ms.rng.Intn(len(matrixGlyphs)))
			}
		}
		a.vis.matrix = ms
	}

	variant := a.vis.variant[visMatrix] % 2

	for i := 0; i < nCols; i++ {
		col := &ms.cols[i]
		f := float64(i) / float64(nCols-1)
		bandVal := bandAt(bands, f)

		// Music-modulated fall speed
		speedMult := 1.0 + 1.2*bass + 0.6*level
		if variant == 1 {
			// In variant 1, FFT bands directly drive speed and length
			speedMult = 0.4 + 2.5*bandVal + 0.8*pulse
		}
		col.headY += col.speed * speedMult

		// Periodic character mutation
		col.swapTick += 1.0 / 60.0
		if col.swapTick > 0.08 {
			col.swapTick = 0
			idx := ms.rng.Intn(col.length)
			col.chars[idx%len(col.chars)] = uint8(ms.rng.Intn(len(matrixGlyphs)))
		}

		tailPx := float64(col.length * rowH)
		if col.headY-tailPx > float64(lh) {
			col.headY = -float64(ms.rng.Intn(rowH * 4))
			col.speed = 1.0 + ms.rng.Float64()*2.2
			if variant == 1 {
				col.length = 6 + int(bandVal*18)
			} else {
				col.length = 8 + ms.rng.Intn(16)
			}
		}

		colX := i * colW
		headRow := int(col.headY / rowH)

		// Draw bright leading glyph
		headScreenY := int(col.headY)
		if headScreenY >= 0 && headScreenY < lh-5 {
			headGlyph := matrixGlyphs[col.chars[0]%uint8(len(matrixGlyphs))]
			headIntensity := 1.6 + 0.4*pulse
			a.drawMatrixGlyph(colX, headScreenY, headGlyph, gray(headIntensity))
		}

		// Draw fading trail
		for t := 1; t < col.length; t++ {
			y := (headRow - t) * rowH
			if y < -5 || y >= lh {
				continue
			}
			fade := math.Pow(1.0-float64(t)/float64(col.length), 1.5)
			intensity := fade * (0.85 + 0.5*bandVal)
			if intensity < 0.08 {
				continue
			}
			glyph := matrixGlyphs[col.chars[t%len(col.chars)]%uint8(len(matrixGlyphs))]
			a.drawMatrixGlyph(colX, y, glyph, gray(intensity))
		}
	}

	a.batch.draw(img)
}

// drawMatrixGlyph appends lit pixels of a 4x5 glyph to the vertex batch.
func (a *App) drawMatrixGlyph(x, y int, glyph [5]uint8, col color.Color) {
	for r := 0; r < 5; r++ {
		row := glyph[r]
		py := y + r
		if (row & 0x8) != 0 {
			a.batch.add(x, py, 1, 1, col)
		}
		if (row & 0x4) != 0 {
			a.batch.add(x+1, py, 1, 1, col)
		}
		if (row & 0x2) != 0 {
			a.batch.add(x+2, py, 1, 1, col)
		}
		if (row & 0x1) != 0 {
			a.batch.add(x+3, py, 1, 1, col)
		}
	}
}
