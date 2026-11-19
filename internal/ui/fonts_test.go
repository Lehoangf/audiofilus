package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Los kanji, kana y hanzi que no tienen Tiny5 ni Jersey salen de Fusion
// Pixel, sin cambiar las medidas de la línea (el texto latino no se mueve).
func TestCJKFallback(t *testing.T) {
	start := time.Now()
	if err := LoadFonts(); err != nil {
		t.Fatal(err)
	}
	if w := text.Advance("初恋", SmallFace); w != text.Advance("初恋", smallBase) {
		t.Errorf("Fusion Pixel was in use before loading in the background")
	}
	pollFonts(true)
	t.Logf("fonts loaded in %v", time.Since(start))
	for _, f := range []struct {
		name       string
		face, base text.Face
	}{{"small", SmallFace, smallBase}, {"big", BigFace, bigBase}} {
		if f.face.Metrics() != f.base.Metrics() {
			t.Errorf("%s: metrics %+v, want the base font's %+v", f.name, f.face.Metrics(), f.base.Metrics())
		}
		for _, s := range []string{"初恋", "ヒカル", "设置"} {
			if w := text.Advance(s, f.face); w <= 0 {
				t.Errorf("%s: %q has no width", f.name, s)
			}
		}
	}
}

// TestVietnameseFontsNativeSupport verifica que las fuentes por defecto utilicen
// las versiones vietnamizadas (-VN) con soporte nativo de caracteres diacríticos.
func TestVietnameseFontsNativeSupport(t *testing.T) {
	if err := LoadFonts(); err != nil {
		t.Fatal(err)
	}
	bigFont, smallFont := defaultFontNames()
	if bigFont != "fonts/Jersey10-Regular-VN.ttf" {
		t.Errorf("bigFont = %q, want fonts/Jersey10-Regular-VN.ttf", bigFont)
	}
	if smallFont != "fonts/Tiny5-Regular-VN.ttf" {
		t.Errorf("smallFont = %q, want fonts/Tiny5-Regular-VN.ttf", smallFont)
	}

	samples := []string{
		"Tiếng Việt",
		"Đang phát nhạc",
		"Bài hát yêu thích",
		"Nơi này có anh",
		"Bầu trời mới",
		"Thoảng hương hoa",
	}
	for _, f := range []struct {
		name string
		face text.Face
	}{{"small", SmallFace}, {"big", BigFace}} {
		for _, s := range samples {
			w := text.Advance(s, f.face)
			if w <= 0 {
				t.Errorf("%s: %q advance is %v, want > 0", f.name, s, w)
			}
		}
	}
}
