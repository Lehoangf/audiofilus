package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigKeepsControls kiểm tra rằng phần "controls" trong config.json
// không còn bị daemon bỏ qua như trước đây.
func TestLoadConfigKeepsControls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{
		"device": {"event_path": "/dev/input/event3", "event_type": 5, "event_code": 1},
		"audio": {"dac_volume": 150, "headphone_gain": 3},
		"actions": {"cpu": {"enabled": true, "governor_on": "powersave"}},
		"controls": {
			"f1_action": "SCREENSHOT",
			"f2_action": "brightness_up",
			"home_action": "exit_es",
			"retroarch_keys": {"menu": 118}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Controls.ControlAction(ButtonF1); got != ActionScreenshot {
		t.Errorf("F1 = %q, muốn %q", got, ActionScreenshot)
	}
	if got := cfg.Controls.ControlAction(ButtonF2); got != ActionBrightnessUp {
		t.Errorf("F2 = %q, muốn %q", got, ActionBrightnessUp)
	}
	if got := cfg.Controls.ControlAction(ButtonHome); got != ActionExitES {
		t.Errorf("Home = %q, muốn %q", got, ActionExitES)
	}
	if got := cfg.Controls.RetroArchKeys.RetroArchKey(ActionRetroArchMenu); got != 118 {
		t.Errorf("menu key = %d, muốn 118", got)
	}
	if cfg.Audio.DACVolume != 150 || cfg.Audio.HeadphoneGain != 3 {
		t.Errorf("audio = %+v, muốn dac=150 gain=3", cfg.Audio)
	}
}

// TestDefaultControls kiểm tra các hành động mặc định khớp với giao diện Brick
// Pro FN Config (mute / LED / RetroArch menu).
func TestDefaultControls(t *testing.T) {
	cfg := DefaultConfig()
	if got := cfg.Controls.ControlAction(ButtonF1); got != ActionMute {
		t.Errorf("F1 mặc định = %q, muốn %q", got, ActionMute)
	}
	if got := cfg.Controls.ControlAction(ButtonF2); got != ActionLEDToggle {
		t.Errorf("F2 mặc định = %q, muốn %q", got, ActionLEDToggle)
	}
	if got := cfg.Controls.ControlAction(ButtonHome); got != ActionRetroArchMenu {
		t.Errorf("Home mặc định = %q, muốn %q", got, ActionRetroArchMenu)
	}
}

// TestNormalizeAction kiểm tra tên_action được chuẩn hóa và tên lạ không bao
// giờ chạy nhầm một lệnh ngoài ý muốn.
func TestNormalizeAction(t *testing.T) {
	cases := map[string]string{
		" mute ":       ActionMute,
		"LED_TOGGLE":   ActionLEDToggle,
		"leds_toggle":  ActionLEDToggle,
		"off":          ActionNone,
		"":             ActionNone,
		"tung_la_gì":   ActionNone,
		"retroarch_ff": ActionRetroArchFF,
	}
	for in, want := range cases {
		if got := normalizeAction(in); got != want {
			t.Errorf("normalizeAction(%q) = %q, muốn %q", in, got, want)
		}
	}
}

// TestRetroArchKeyFallbacks kiểm tra config.json cũ (không có retroarch_keys)
// vẫn dùng được mã phím mặc định.
func TestRetroArchKeyFallbacks(t *testing.T) {
	var keys RetroArchKeys
	cases := map[string]uint16{
		ActionRetroArchMenu: keyF1,
		ActionQuickMenu:     keyHomepage,
		ActionRetroArchSave: keyF2,
		ActionRetroArchFF:   keyF,
	}
	for action, want := range cases {
		if got := keys.RetroArchKey(action); got != want {
			t.Errorf("%s = %d, muốn %d", action, got, want)
		}
	}
}

// TestButtonForKey kiểm tra ba mã Home mà các bản firmware khác nhau dùng đều
// được nhận diện là cùng một nút.
func TestButtonForKey(t *testing.T) {
	cases := map[uint16]ButtonKind{
		keyF1:       ButtonF1,
		keyF2:       ButtonF2,
		keyHomepage: ButtonHome,
		keyMenu:     ButtonHome,
		keyHome:     ButtonHome,
	}
	for code, want := range cases {
		got, ok := buttonForKey(code)
		if !ok || got != want {
			t.Errorf("buttonForKey(%d) = (%v, %v), muốn (%v, true)", code, got, ok, want)
		}
	}
	if _, ok := buttonForKey(30); ok {
		t.Errorf("phím không liên quan (30) không được nhận diện là nút phụ")
	}
}
