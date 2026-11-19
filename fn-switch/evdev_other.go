//go:build !linux

package main

import (
	"errors"
	"log"
	"os"
)

// Các hàm evdev chỉ có ý nghĩa trên Linux. Stub này giúp `go test ./...` chạy
// được trên PC (Windows/macOS) để kiểm tra config và ánh xạ nút.
type InputDeviceInfo struct {
	Name     string
	Path     string
	HasSW    bool
	SwBitmap string
}

// ListInputDevices trả về danh sách rỗng trên nền tảng không phải Linux.
func ListInputDevices() ([]InputDeviceInfo, error) { return nil, nil }

// ReadInputEvent không hỗ trợ trên nền tảng không phải Linux.
func ReadInputEvent(*os.File) (uint16, uint16, int32, error) {
	return 0, 0, 0, errors.New("evdev chỉ có trên Linux")
}

// getSwitchState không hỗ trợ trên nền tảng không phải Linux.
func getSwitchState(uintptr, int) (bool, error) {
	return false, errors.New("evdev chỉ có trên Linux")
}

// FindMatchingDevice không hỗ trợ trên nền tảng không phải Linux.
func FindMatchingDevice(*Config) (string, error) {
	return "", errors.New("evdev chỉ có trên Linux")
}

// RunProbeMode chỉ in lời nhắc trên nền tảng không phải Linux.
func RunProbeMode(string) { log.Printf("[PROBE] Chế độ dò tìm chỉ chạy trên Linux") }

// ListenEvents chỉ in lời nhắc trên nền tảng không phải Linux.
func ListenEvents(*Config, *HardwareManager) error {
	return errors.New("daemon fn-switch chỉ chạy trên Linux")
}
