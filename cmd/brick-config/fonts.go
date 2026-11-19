package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"image"
	"image/color"
	"io"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/juanvillacortac/audiofilus/internal/ui/assets"
)

var (
	TitleFace text.Face
	BodyFace  text.Face
	SmallFace text.Face
)

type crispFace struct{ font.Face }

func (c crispFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	dot.X, dot.Y = fixed.I(dot.X.Round()), fixed.I(dot.Y.Round())
	dr, mask, mp, adv, ok := c.Face.Glyph(dot, r)
	if !ok {
		return dr, mask, mp, adv, ok
	}
	out := image.NewAlpha(image.Rect(0, 0, dr.Dx(), dr.Dy()))
	for y := 0; y < dr.Dy(); y++ {
		for x := 0; x < dr.Dx(); x++ {
			if _, _, _, a := mask.At(mp.X+x, mp.Y+y).RGBA(); a >= 0x8000 {
				out.Pix[y*out.Stride+x] = 0xff
			}
		}
	}
	return dr, out, image.Point{}, adv, true
}

func parseFont(name string) (*opentype.Font, error) {
	data, err := assets.FS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if data, err = io.ReadAll(zr); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return opentype.Parse(data)
}

func initFonts() error {
	// 1. Title Face: Jersey 10 VN hoặc fallback
	titleName := "fonts/Jersey10-Regular-VN.ttf"
	if _, err := assets.FS.ReadFile(titleName); err != nil {
		titleName = "fonts/Jersey10-Regular.ttf"
	}
	tf, err := parseFont(titleName)
	if err == nil {
		face, err := opentype.NewFace(tf, &opentype.FaceOptions{
			Size:    1400.0 / 75.0, // native pixel grid
			DPI:     72,
			Hinting: font.HintingNone,
		})
		if err == nil {
			TitleFace = text.NewGoXFace(crispFace{face})
		}
	}

	// 2. Body Face: Tiny5 VN hoặc Retron-2000
	bodyName := "fonts/Tiny5-Regular-VN.ttf"
	if _, err := assets.FS.ReadFile(bodyName); err != nil {
		bodyName = "fonts/Tiny5-Regular.ttf"
	}
	bf, err := parseFont(bodyName)
	if err == nil {
		face, err := opentype.NewFace(bf, &opentype.FaceOptions{
			Size:    8, // 8px native size
			DPI:     72,
			Hinting: font.HintingNone,
		})
		if err == nil {
			BodyFace = text.NewGoXFace(crispFace{face})
			SmallFace = BodyFace
		}
	}

	// Nếu load từ assets lỗi thì fallback sang Retron-2000
	if BodyFace == nil {
		rf, err := parseFont("fonts/SVN-Retron-2000.ttf")
		if err == nil {
			face, _ := opentype.NewFace(rf, &opentype.FaceOptions{
				Size:    10,
				DPI:     72,
				Hinting: font.HintingNone,
			})
			BodyFace = text.NewGoXFace(crispFace{face})
			SmallFace = BodyFace
		}
	}

	if TitleFace == nil {
		TitleFace = BodyFace
	}
	return nil
}

// drawText vẽ chuỗi text tại tọa độ x, y
func drawText(dst *ebiten.Image, s string, face text.Face, x, y int, c color.Color) {
	if face == nil || s == "" {
		return
	}
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, face, op)
}

// drawTextShadow vẽ chữ kèm bóng đổ 1px
func drawTextShadow(dst *ebiten.Image, s string, face text.Face, x, y int, c, shadow color.Color) {
	drawText(dst, s, face, x+1, y+1, shadow)
	drawText(dst, s, face, x, y, c)
}

// textWidth trả về độ rộng của text tính theo pixel
func textWidth(s string, face text.Face) int {
	if face == nil || s == "" {
		return 0
	}
	return int(text.Advance(s, face) + 0.5)
}
