//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMuOSTrimUIBrickLED(t *testing.T) {
	temp := t.TempDir()
	ledFile := filepath.Join(temp, "max_scale")
	_ = os.WriteFile(ledFile, []byte("180"), 0o644)

	m := &muos{ledPath: ledFile}
	if err := m.EnterSleep(true); err != nil {
		t.Fatalf("EnterSleep: %v", err)
	}
	val, _ := os.ReadFile(ledFile)
	if string(val) != "0" {
		t.Errorf("expected LED max_scale 0 during sleep, got %s", val)
	}

	if err := m.ExitSleep(); err != nil {
		t.Fatalf("ExitSleep: %v", err)
	}
	val, _ = os.ReadFile(ledFile)
	if string(val) != "180" {
		t.Errorf("expected LED max_scale restored to 180, got %s", val)
	}
}

func TestDebounceFnSwitch(t *testing.T) {
	sw := newFnSwitchWatcher(nil)
	if sw == nil {
		t.Fatal("expected non-nil watcher")
	}
}
