package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestNowPlayingVizCycleAndDraw(t *testing.T) {
	app := &App{}
	modes := []string{"led", "smooth", "wave", "mirror", "off"}
	for _, mode := range modes {
		app.cfg.UI.NowPlayingViz = mode
		img := ebiten.NewImage(320, 240)
		app.drawSpectrum(img, 10, 150, 300, 40)
	}
	app.cfg.UI.NowPlayingViz = "led"
	app.cycleNowPlayingViz()
	if app.cfg.UI.NowPlayingViz != "smooth" {
		t.Errorf("expected smooth, got %s", app.cfg.UI.NowPlayingViz)
	}
}
