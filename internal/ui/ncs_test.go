package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestNCSVisualizerDrawNoPanic(t *testing.T) {
	app := &App{}
	img := ebiten.NewImage(320, 240)
	for variant := 0; variant < 3; variant++ {
		app.vis.variant[visNCS] = variant
		app.drawNCS(img)
	}
}
