//go:build !linux

package main

import "fmt"

// keyInjector là stub trên các hệ điều hành không có uinput. Code dùng chung
// vẫn biên dịch và chạy test được trên PC; chỉ hành động RetroArch là bỏ qua.
type keyInjector struct{}

// newKeyInjector luôn trả về lỗi trên nền tảng không hỗ trợ uinput.
func newKeyInjector([]uint16) (*keyInjector, error) {
	return nil, fmt.Errorf("uinput chỉ có trên Linux")
}

// Tap không làm gì trên nền tảng không hỗ trợ uinput.
func (*keyInjector) Tap(uint16) error { return nil }

// Close không làm gì trên nền tảng không hỗ trợ uinput.
func (*keyInjector) Close() error { return nil }
