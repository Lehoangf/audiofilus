package main

import "strings"

// Các loại sự kiện input của Linux (từ linux/input-event-codes.h). Đặt chung ở
// đây để cả phần đọc evdev lẫn phần tiêm phím uinput dùng cùng một bộ mã.
const (
	evSyn = 0x00
	evKey = 0x01
	evSw  = 0x05

	synReport = 0

	// keyF1/keyF2 là các nút phụ vật lý trên Brick Pro.
	keyF1 uint16 = 59 // KEY_F1
	keyF2 uint16 = 60 // KEY_F2

	// Home được các bản Knulli/MuOS khác nhau gán cho một trong ba mã này; chấp
	// nhận cả ba để không phải đoán từng bản firmware.
	keyHomepage uint16 = 172 // KEY_HOMEPAGE
	keyMenu     uint16 = 139 // KEY_MENU
	keyHome     uint16 = 102 // KEY_HOME

	keyF uint16 = 33 // KEY_F, mặc định của RetroArch "Toggle Fast-Forward".

	// fnSwitchCode là SW_TABLET_MODE, công tắc gạt FN trên Brick Pro.
	fnSwitchCode = 1
)

// Các tên hành động mà giao diện Brick Pro FN Config ghi vào config.json.
const (
	ActionNone           = "none"
	ActionMute           = "mute"
	ActionScreenshot     = "screenshot"
	ActionBrightnessDown = "brightness_down"
	ActionBrightnessUp   = "brightness_up"
	ActionLEDToggle      = "led_toggle"
	ActionRetroArchFF    = "retroarch_ff"
	ActionRetroArchSave  = "retroarch_save"
	ActionRetroArchMenu  = "retroarch_menu"
	ActionQuickMenu      = "quick_menu"
	ActionExitES         = "exit_es"
	ActionCustom         = "custom"
)

// aliasAction chuẩn hóa vài tên hay gõ nhầm/tên cũ của giao diện.
var aliasAction = map[string]string{
	"":              ActionNone,
	"off":           ActionNone,
	"leds_toggle":   ActionLEDToggle,
	"brightness_up": ActionBrightnessUp,
}

// ButtonKind xác định một trong ba nút phụ có thể gán hành động.
type ButtonKind int

const (
	// ButtonF1 là nút F1 trên thân máy.
	ButtonF1 ButtonKind = iota
	// ButtonF2 là nút F2 trên thân máy.
	ButtonF2
	// ButtonHome là phím Home/ Menu.
	ButtonHome
)

// String trả về tên ngắn gọn để ghi log.
func (b ButtonKind) String() string {
	switch b {
	case ButtonF1:
		return "F1"
	case ButtonF2:
		return "F2"
	case ButtonHome:
		return "HOME"
	}
	return "?"
}

// normalizeAction đưa một tên hành động về dạng thường, bỏ khoảng trắng và
// gộp các tên cũ. Giao diện và daemon vì thế không cần phải khớp từng chữ hoa.
func normalizeAction(raw string) string {
	a := strings.ToLower(strings.TrimSpace(raw))
	if alias, ok := aliasAction[a]; ok {
		return alias
	}
	switch a {
	case ActionMute, ActionScreenshot, ActionBrightnessDown, ActionBrightnessUp,
		ActionLEDToggle, ActionRetroArchFF, ActionRetroArchSave,
		ActionRetroArchMenu, ActionQuickMenu, ActionExitES, ActionCustom:
		return a
	}
	return ActionNone
}

// isHomeKey nhận diện ba mã phím mà các bản firmware khác nhau dùng cho Home.
func isHomeKey(code uint16) bool {
	return code == keyHomepage || code == keyMenu || code == keyHome
}

// buttonForKey trả về nút phụ tương ứng với một mã EV_KEY, và ok=false nếu mã đó
// không phải nút được quan tâm.
func buttonForKey(code uint16) (ButtonKind, bool) {
	switch {
	case code == keyF1:
		return ButtonF1, true
	case code == keyF2:
		return ButtonF2, true
	case isHomeKey(code):
		return ButtonHome, true
	}
	return ButtonF1, false
}
