package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/juanvillacortac/audiofilus/internal/config"
)

func TestMatrixAndWarpDrawNoPanic(t *testing.T) {
	app := &App{}
	img := ebiten.NewImage(320, 240)
	app.drawMatrix(img)
	app.drawWarp(img)
}

// TestVisualizerVariantPersistence comprueba que las variantes y cámaras
// elegidas se guarden en la configuración y se restauren al iniciar la
// aplicación.
func TestVisualizerVariantPersistence(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	_ = os.MkdirAll(dataDir, 0o755)
	paths := config.Paths{
		Base:   dir,
		Config: filepath.Join(dir, "audiofilus.json"),
		Data:   dataDir,
	}

	cfg := config.Default()
	cfg.UI.Visualizer = "bars"
	cfg.UI.VisualVariants = map[string]int{
		"bars":  1,
		"ncs":   2,
		"drift": 1,
		"lake":  2,
	}
	if err := config.Save(paths, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	app, err := NewApp(paths, true)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	if app.vis.variant[visBars] != 1 {
		t.Errorf("bars variant = %d, want 1", app.vis.variant[visBars])
	}
	if app.vis.variant[visNCS] != 2 {
		t.Errorf("ncs variant = %d, want 2", app.vis.variant[visNCS])
	}
	if app.vis.variant[visDrift] != 1 {
		t.Errorf("drift variant = %d, want 1", app.vis.variant[visDrift])
	}
	if app.vis.variant[visLake] != 2 {
		t.Errorf("lake variant = %d, want 2", app.vis.variant[visLake])
	}

	// Comprueba que los visualizadores 3D tomen la cámara al crearse.
	img := ebiten.NewImage(320, 240)
	app.drawDriftScene(img)
	if app.drift == nil || app.drift.cam != 1 {
		t.Errorf("drift cam = %v, want 1", app.drift)
	}
	app.drawLakeScene(img)
	if app.lake == nil || app.lake.cam != 2 {
		t.Errorf("lake cam = %v, want 2", app.lake)
	}

	// Comprueba que al cambiar y guardar se refleje en cfg.
	app.vis.variant[visBars] = 0
	app.saveVisualVariant(visBars)
	if app.cfg.UI.VisualVariants["bars"] != 0 {
		t.Errorf("saved bars variant = %d, want 0", app.cfg.UI.VisualVariants["bars"])
	}
}
