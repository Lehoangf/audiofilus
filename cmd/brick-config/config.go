package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// AppConfig cấu hình đầy đủ cho Brick Pro FN Config. Audio được giữ lại dù giao
// diện không chỉnh trực tiếp: nếu thiếu, mỗi lần bấm START Lưu sẽ xóa mức DAC
// và gain tai nghe mà fn-daemon đã đặt cho loa trong.
type AppConfig struct {
	Device   DeviceConfig   `json:"device"`
	Audio    AudioConfig    `json:"audio"`
	Actions  ActionsConfig  `json:"actions"`
	Controls ControlsConfig `json:"controls"`
}

// AudioConfig là các mức phần cứng do fn-daemon quản lý.
type AudioConfig struct {
	DACVolume     int `json:"dac_volume"`
	HeadphoneGain int `json:"headphone_gain"`
}

type DeviceConfig struct {
	EventPath  string `json:"event_path"`
	DeviceName string `json:"device_name"`
	EventType  uint16 `json:"event_type"`
	EventCode  int    `json:"event_code"`
	Invert     bool   `json:"invert"`
}

type ActionsConfig struct {
	CPU            CPUAction        `json:"cpu"`
	Brightness     BrightnessAction `json:"brightness"`
	Volume         VolumeAction     `json:"volume"`
	LEDs           LEDAction        `json:"leds"`
	CustomCommands CustomAction     `json:"custom_commands"`
}

type CPUAction struct {
	Enabled    bool   `json:"enabled"`
	GovernorOn string `json:"governor_on"` // "powersave", "schedutil", "performance"
}

type BrightnessAction struct {
	Enabled   bool `json:"enabled"`
	PercentOn int  `json:"percent_on"` // 0 - 100%
}

type VolumeAction struct {
	Enabled    bool `json:"enabled"`
	MuteOn     bool `json:"mute_on"`
	PercentOn  int  `json:"percent_on"`  // 0 - 100%
	PauseMedia bool `json:"pause_media"` // Tạm dừng phát nhạc khi gạt ON
}

type LEDAction struct {
	Enabled    bool `json:"enabled"`
	ScaleOn    int  `json:"scale_on"`     // Top strip (0-255)
	ScaleLR    int  `json:"scale_lr"`     // Analog Joysticks Halo (0-40)
	ScaleRear  int  `json:"scale_rear"`   // Shoulder buttons (0-40)
	ScaleF1F2  int  `json:"scale_f1f2"`   // F1/F2 keys (0-40)
	BreathOff  bool `json:"breath_off"`   // Tắt hiệu ứng thở khi ON
	TurnOffAll bool `json:"turn_off_all"` // Tắt hẳn toàn bộ đèn
}

type CustomAction struct {
	Enabled    bool   `json:"enabled"`
	OnCommand  string `json:"on_command"`
	OffCommand string `json:"off_command"`
}

type ControlsConfig struct {
	F1Action   string `json:"f1_action"`   // "none", "mute", "screenshot", "brightness_down", "retroarch_ff", "custom"
	F2Action   string `json:"f2_action"`   // "none", "led_toggle", "brightness_up", "retroarch_save", "custom"
	HomeAction string `json:"home_action"` // "none", "retroarch_menu", "exit_es", "custom"

	// Các trường dưới đây giao diện chưa hiển thị nhưng phải đi vòng tròn khi
	// lưu file, nếu không daemon sẽ mất lệnh custom và mã phím RetroArch đã đặt.
	F1Command     string        `json:"f1_command,omitempty"`
	F2Command     string        `json:"f2_command,omitempty"`
	HomeCommand   string        `json:"home_command,omitempty"`
	RetroArchKeys RetroArchKeys `json:"retroarch_keys,omitempty"`
}

// RetroArchKeys là mã phím daemon tiêm qua uinput (0 = dùng mặc định).
type RetroArchKeys struct {
	Menu        uint16 `json:"menu,omitempty"`
	QuickMenu   uint16 `json:"quick_menu,omitempty"`
	SaveState   uint16 `json:"save_state,omitempty"`
	FastForward uint16 `json:"fast_forward,omitempty"`
}

// ConfigFilePath trả về đường dẫn tới config.json
func ConfigFilePath() string {
	target := "/userdata/system/fn_switch/config.json"
	if _, err := os.Stat("/userdata/system"); err == nil {
		return target
	}
	// Fallback local khi test trên máy tính
	return "config.json"
}

// DefaultAppConfig trả về cấu hình mặc định tối ưu cho TrimUI Brick Pro
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		Device: DeviceConfig{
			EventPath:  "/dev/input/event3",
			DeviceName: "TRIMUI Brick Pro Controller",
			EventType:  5, // EV_SW
			EventCode:  1, // SW_TABLET_MODE
			Invert:     false,
		},
		Audio: AudioConfig{
			DACVolume:     140,
			HeadphoneGain: 0,
		},
		Actions: ActionsConfig{
			CPU: CPUAction{
				Enabled:    true,
				GovernorOn: "powersave",
			},
			Brightness: BrightnessAction{
				Enabled:   true,
				PercentOn: 20,
			},
			Volume: VolumeAction{
				Enabled:    true,
				MuteOn:     true,
				PercentOn:  0,
				PauseMedia: true,
			},
			LEDs: LEDAction{
				Enabled:    true,
				ScaleOn:    0,
				ScaleLR:    0,
				ScaleRear:  0,
				ScaleF1F2:  0,
				BreathOff:  true,
				TurnOffAll: true,
			},
			CustomCommands: CustomAction{
				Enabled:    false,
				OnCommand:  "",
				OffCommand: "",
			},
		},
		Controls: ControlsConfig{
			F1Action:   "mute",
			F2Action:   "led_toggle",
			HomeAction: "retroarch_menu",
		},
	}
}

// LoadConfig tải file cấu hình từ đĩa
func LoadConfig() (*AppConfig, error) {
	path := ConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		cfg := DefaultAppConfig()
		_ = SaveConfig(cfg)
		return cfg, nil
	}

	cfg := DefaultAppConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveConfig lưu cấu hình xuống đĩa và gửi tín hiệu cập nhật cho daemon
func SaveConfig(cfg *AppConfig) error {
	path := ConfigFilePath()
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, path); err != nil {
		return err
	}

	// Gửi tín hiệu reload cho daemon hoặc chạy script khởi động lại
	_ = exec.Command("pkill", "-HUP", "fn-daemon").Run()
	return nil
}
