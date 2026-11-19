package main

import (
	"encoding/json"
	"os"
)

// Config chứa toàn bộ thiết lập của fn-daemon.
//
// Controls là phần do ứng dụng Brick Pro FN Config ghi vào cùng một file. Trước
// đây daemon không khai báo trường này nên json.Unmarshal âm thầm bỏ qua mọi
// hành động F1/F2/Home người dùng chọn trong giao diện.
type Config struct {
	Device   DeviceConfig   `json:"device"`
	Audio    AudioConfig    `json:"audio"`
	Actions  ActionsConfig  `json:"actions"`
	Controls ControlsConfig `json:"controls"`
}

// AudioConfig cấu hình mức phần cứng mà daemon áp lại mỗi lần khởi động. Knulli
// không đặt mức DAC (giữ nguyên thứ alsactl lưu lại), nên đây là chỗ duy nhất để
// chỉnh độ to của loa trong mà không phải sửa code.
type AudioConfig struct {
	// DACVolume: mức "DAC volume" của codec (0-255, 159 = 0 dB theo amixer).
	// Mặc định 140, đúng mức device tree để lại (-14.25 dB). Tăng thêm chỉ có
	// nguy cơ vỡ tiếng: PipeWire còn nhân thêm âm lượng mềm của sink.
	DACVolume int `json:"dac_volume"`
	// HeadphoneGain: mức "Headphone Volume" (0-7). Mặc định 0: trên Brick Pro
	// đây là mức làm loa trong to (TrimUI đảo ngược cổng tai nghe, nên amixer ghi
	// -42 dB nhưng thực tế ngược lại). 7 làm loa trong nhỏ đi rất nhiều.
	HeadphoneGain int `json:"headphone_gain"`
}

// DeviceConfig cấu hình thiết bị đầu vào để bắt tín hiệu FN switch.
type DeviceConfig struct {
	// EventPath: đường dẫn thiết bị (/dev/input/eventX). Nếu để "auto", daemon sẽ tự quét.
	EventPath string `json:"event_path"`
	// DeviceName: tên thiết bị (ví dụ "gpio-keys", "trimui-keys") khi quét auto.
	DeviceName string `json:"device_name"`
	// EventType: 5 (EV_SW) hoặc 1 (EV_KEY). 0 = tự nhận.
	EventType uint16 `json:"event_type"`
	// EventCode: mã code của công tắc (ví dụ 0 = SW_LID). -1 = tự nhận bất kỳ switch nào đổi trạng thái.
	EventCode int `json:"event_code"`
	// Invert: đảo ngược trạng thái (mặc định 1 là ON, 0 là OFF).
	Invert bool `json:"invert"`
}

// ActionsConfig chứa các hành động tinh chỉnh khi gạt ON.
type ActionsConfig struct {
	CPU            CPUAction        `json:"cpu"`
	Brightness     BrightnessAction `json:"brightness"`
	Volume         VolumeAction     `json:"volume"`
	LEDs           LEDAction        `json:"leds"`
	CustomCommands CustomAction     `json:"custom_commands"`
}

// CPUAction cấu hình CPU Governor khi gạt ON.
type CPUAction struct {
	Enabled    bool   `json:"enabled"`
	GovernorOn string `json:"governor_on"` // ví dụ: "powersave", "performance"
}

// BrightnessAction cấu hình độ sáng màn hình khi gạt ON.
type BrightnessAction struct {
	Enabled   bool `json:"enabled"`
	PercentOn int  `json:"percent_on"` // 0 - 100%
}

// VolumeAction cấu hình âm thanh khi gạt ON.
type VolumeAction struct {
	Enabled    bool `json:"enabled"`
	MuteOn     bool `json:"mute_on"`
	PercentOn  int  `json:"percent_on"` // 0 - 100%
	PauseMedia bool `json:"pause_media"`
}

// LEDAction cấu hình đèn LED RGB khi gạt ON.
type LEDAction struct {
	Enabled bool `json:"enabled"`
	ScaleOn int  `json:"scale_on"` // 0 = tắt hẳn, 255 = sáng tối đa
}

// CustomAction lệnh shell tùy biến chạy thêm khi gạt ON / OFF.
type CustomAction struct {
	Enabled    bool   `json:"enabled"`
	OnCommand  string `json:"on_command"`
	OffCommand string `json:"off_command"`
}

// ControlsConfig ánh xạ ba nút phụ (F1, F2, Home) tới một hành động. Các tên hành
// động phải khớp với danh sách trong cmd/brick-config/ui.go, nếu không daemon sẽ
// coi như "none" thay vì bỏ qua một cách im lặng.
//
// F1Command/F2Command/HomeCommand chỉ dùng khi hành động là "custom".
// RetroArchKeys là mã phím daemon tiêm qua uinput cho các hành động RetroArch;
// 0 nghĩa là dùng mã mặc định, nên một file config cũ (không có trường này) vẫn
// chạy đúng.
type ControlsConfig struct {
	F1Action   string `json:"f1_action"`
	F2Action   string `json:"f2_action"`
	HomeAction string `json:"home_action"`

	F1Command   string `json:"f1_command,omitempty"`
	F2Command   string `json:"f2_command,omitempty"`
	HomeCommand string `json:"home_command,omitempty"`

	RetroArchKeys RetroArchKeys `json:"retroarch_keys,omitempty"`
}

// RetroArchKeys chứa mã phím Linux (EV_KEY) mặc định mà RetroArch dùng cho menu,
// lưu nhanh và tua nhanh. Có thể đổi trong config.json nếu RetroArch được remap.
type RetroArchKeys struct {
	Menu        uint16 `json:"menu,omitempty"`         // mặc định KEY_F1 (59)
	QuickMenu   uint16 `json:"quick_menu,omitempty"`   // mặc định KEY_HOMEPAGE (172)
	SaveState   uint16 `json:"save_state,omitempty"`   // mặc định KEY_F2 (60)
	FastForward uint16 `json:"fast_forward,omitempty"` // mặc định KEY_F (33)
}

// DefaultConfig trả về cấu hình mặc định (Power Saving: hạ CPU, giảm sáng, tắt LED).
func DefaultConfig() *Config {
	return &Config{
		Device: DeviceConfig{
			EventPath:  "auto",
			DeviceName: "",
			EventType:  0, // tự nhận
			EventCode:  -1,
			Invert:     false,
		},
		Audio: AudioConfig{
			DACVolume:     defaultDACVolume,
			HeadphoneGain: defaultHeadphoneGain,
		},
		Actions: ActionsConfig{
			CPU: CPUAction{
				Enabled:    true,
				GovernorOn: "powersave",
			},
			Brightness: BrightnessAction{
				Enabled:   true,
				PercentOn: 25, // Hạ độ sáng xuống 25% khi ON
			},
			Volume: VolumeAction{
				Enabled:   false, // Mặc định không can thiệp volume
				MuteOn:    false,
				PercentOn: 0,
			},
			LEDs: LEDAction{
				Enabled: true,
				ScaleOn: 0, // Tắt dải đèn LED khi ON để tiết kiệm pin
			},
			CustomCommands: CustomAction{
				Enabled:    false,
				OnCommand:  "",
				OffCommand: "",
			},
		},
		Controls: ControlsConfig{
			F1Action:   ActionMute,
			F2Action:   ActionLEDToggle,
			HomeAction: ActionRetroArchMenu,
		},
	}
}

// RetroArchKey trả về mã phím đã cấu hình cho một hành động RetroArch, rơi về
// mặc định khi người dùng chưa khai báo (hoặc ghi 0 trong config.json).
func (k RetroArchKeys) RetroArchKey(action string) uint16 {
	switch action {
	case ActionRetroArchMenu:
		if k.Menu != 0 {
			return k.Menu
		}
		return keyF1
	case ActionQuickMenu:
		if k.QuickMenu != 0 {
			return k.QuickMenu
		}
		return keyHomepage
	case ActionRetroArchSave:
		if k.SaveState != 0 {
			return k.SaveState
		}
		return keyF2
	case ActionRetroArchFF:
		if k.FastForward != 0 {
			return k.FastForward
		}
		return keyF
	}
	return 0
}

// ControlAction trả về hành động đã map cho một nút, đã chuẩn hóa chữ thường
// và bỏ khoảng trắng. Một giá trị không hợp lệ trở thành "none" để daemon không
// bao giờ chạy nhầm lệnh khi config.json bị sửa tay.
func (c ControlsConfig) ControlAction(btn ButtonKind) string {
	var raw string
	switch btn {
	case ButtonF1:
		raw = c.F1Action
	case ButtonF2:
		raw = c.F2Action
	case ButtonHome:
		raw = c.HomeAction
	}
	return normalizeAction(raw)
}

// ControlCommand trả về lệnh shell của nút khi hành động là "custom".
func (c ControlsConfig) ControlCommand(btn ButtonKind) string {
	switch btn {
	case ButtonF1:
		return c.F1Command
	case ButtonF2:
		return c.F2Command
	case ButtonHome:
		return c.HomeCommand
	}
	return ""
}

// LoadConfig nạp cấu hình từ đường dẫn file JSON, nếu không có sẽ dùng mặc định.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		cfg := DefaultConfig()
		_ = SaveConfig(path, cfg)
		return cfg, nil
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveConfig lưu cấu hình ra file JSON.
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
