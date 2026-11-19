package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestProgressStyleCycleAndNyanDraw(t *testing.T) {
	_ = LoadFonts()
	app := &App{duration: 200, shownPos: 50}
	img := ebiten.NewImage(320, 240)
	styles := []string{"classic", "retro", "thin", "pill", "nyancat"}
	for _, s := range styles {
		app.cfg.UI.ProgressStyle = s
		app.drawProgress(img, 10, 200, 300)
	}
	app.cfg.UI.ProgressStyle = "classic"
	app.cycleProgressStyle()
	if app.cfg.UI.ProgressStyle != "retro" {
		t.Errorf("expected retro, got %s", app.cfg.UI.ProgressStyle)
	}
}
