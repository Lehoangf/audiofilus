package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Theme chứa bảng màu Pixel Art phong cách Audiofilus
type Theme struct {
	Bg          color.RGBA // Nền chính (#0D0B17)
	PanelBg     color.RGBA // Nền thẻ panel (#1A162B)
	PanelBorder color.RGBA // Viền thẻ mặc định (#2B2544)
	PanelFocus  color.RGBA // Nền thẻ khi hover/focus (#27213F)
	Accent      color.RGBA // Màu điểm nhấn Cyber Turquoise (#4FE3C1)
	Accent2     color.RGBA // Màu điểm nhấn Amber (#FFB000)
	Text        color.RGBA // Chữ sáng (#F1ECFF)
	TextDim     color.RGBA // Chữ phụ/mờ (#8A82A8)
	Success     color.RGBA // Màu trạng thái bật (#00E676)
	Danger      color.RGBA // Màu trạng thái cảnh báo/tắt (#FF4B6E)
	SliderTrack color.RGBA // Rãnh thanh trượt (#342E52)
	SliderFill  color.RGBA // Phần thanh trượt đã chọn (#4FE3C1)
}

var DefaultTheme = Theme{
	Bg:          color.RGBA{0x0d, 0x0b, 0x17, 0xff},
	PanelBg:     color.RGBA{0x18, 0x14, 0x27, 0xff},
	PanelBorder: color.RGBA{0x2b, 0x25, 0x44, 0xff},
	PanelFocus:  color.RGBA{0x26, 0x20, 0x3d, 0xff},
	Accent:      color.RGBA{0x4f, 0xe3, 0xc1, 0xff},
	Accent2:     color.RGBA{0xff, 0xb0, 0x00, 0xff},
	Text:        color.RGBA{0xf1, 0xec, 0xff, 0xff},
	TextDim:     color.RGBA{0x8a, 0x82, 0xa8, 0xff},
	Success:     color.RGBA{0x00, 0xe6, 0x76, 0xff},
	Danger:      color.RGBA{0xff, 0x4b, 0x6e, 0xff},
	SliderTrack: color.RGBA{0x32, 0x2b, 0x4f, 0xff},
	SliderFill:  color.RGBA{0x4f, 0xe3, 0xc1, 0xff},
}

var whitePx *ebiten.Image

func whitePixel() *ebiten.Image {
	if whitePx == nil {
		img := ebiten.NewImage(3, 3)
		img.Fill(color.White)
		whitePx = img.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
	}
	return whitePx
}

// rect vẽ hình chữ nhật đặc 1 màu
func rect(dst *ebiten.Image, x, y, w, h int, c color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(w), float64(h))
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(c)
	dst.DrawImage(whitePixel(), op)
}

// frame vẽ khung viền chữ nhật 1px
func frame(dst *ebiten.Image, x, y, w, h int, c color.Color) {
	rect(dst, x, y, w, 1, c)
	rect(dst, x, y+h-1, w, 1, c)
	rect(dst, x, y, 1, h, c)
	rect(dst, x+w-1, y, 1, h, c)
}

// panel vẽ hình chữ nhật bo góc pixel 1px chuẩn retro
func panel(dst *ebiten.Image, x, y, w, h int, c color.Color) {
	rect(dst, x+1, y, w-2, h, c)
	rect(dst, x, y+1, 1, h-2, c)
	rect(dst, x+w-1, y+1, 1, h-2, c)
}

// marching vẽ đường viền nét đứt chuyển động theo thời gian (hiệu ứng focus)
func marching(dst *ebiten.Image, x, y, w, h int, tick int, c color.Color) {
	off := (tick / 4) % 4
	n := 2*(w+h) - 4
	for i := 0; i < n; i++ {
		if (i+off)%4 >= 2 {
			continue
		}
		var px, py int
		switch {
		case i < w:
			px, py = x+i, y
		case i < w+h-1:
			px, py = x+w-1, y+i-w+1
		case i < 2*w+h-2:
			px, py = x+w-1-(i-w-h+2), y+h-1
		default:
			px, py = x, y+h-1-(i-2*w-h+3)
		}
		rect(dst, px, py, 1, 1, c)
	}
}
