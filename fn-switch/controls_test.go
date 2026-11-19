package main

import "testing"

// TestRunControlDebounce kiểm tra một lần bấm vật lý không bị xử lý hai lần khi
// cùng một phím xuất hiện trên hai thiết bị input (event3 lẫn event0).
func TestRunControlDebounce(t *testing.T) {
	h := &HardwareManager{}
	// "custom" không có lệnh nên không gọi tiến trình ngoài nào trong test.
	cfg := &Config{Controls: ControlsConfig{F1Action: ActionCustom}}

	h.RunControl(ButtonF1, cfg)
	first := h.lastPress[ButtonF1]
	if first.IsZero() {
		t.Fatal("lần bấm đầu tiên không được ghi nhận")
	}
	h.RunControl(ButtonF1, cfg)
	if got := h.lastPress[ButtonF1]; !got.Equal(first) {
		t.Errorf("lần bấm thứ hai bị xử lý dù nằm trong ngưỡng debounce")
	}
}

// TestRunControlIgnoresNone kiểm tra hành động "none" không đụng vào trạng thái
// nút, để một phím chưa map không bao giờ gây tác dụng phụ.
func TestRunControlIgnoresNone(t *testing.T) {
	h := &HardwareManager{}
	cfg := &Config{Controls: ControlsConfig{HomeAction: ActionNone}}

	h.RunControl(ButtonHome, cfg)
	if !h.lastPress[ButtonHome].IsZero() {
		t.Errorf("hành động none vẫn ghi nhận lần bấm")
	}
}
