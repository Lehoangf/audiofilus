//go:build linux

package main

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// Ioctl của uinput (linux/uinput.h). Các giá trị được tính theo công thức
// _IOW/_IO nên không cần cgo hay header C.
const (
	uiSetEvBit   = 0x40045504 // _IOW('U', 4, int)
	uiSetKeyBit  = 0x40045505 // _IOW('U', 5, int)
	uiDevSetup   = 0x405c5503 // _IOW('U', 3, struct uinput_setup), 92 bytes
	uiDevCreate  = 0x5501     // _IO('U', 1)
	uiDevDestroy = 0x5502     // _IO('U', 2)

	inputEventSize = 24 // tv_sec(8) + tv_usec(8) + type(2) + code(2) + value(4)
	busUSB         = 0x03
)

// uinputSetup khớp struct uinput_setup: input_id(8) + name[80] + ff_effects_max(4).
type uinputSetup struct {
	BusType uint16
	Vendor  uint16
	Product uint16
	Version uint16
	Name    [80]byte
	Effects uint32
}

// inputEvent khớp struct input_event trên Linux 64-bit (timeval 16 byte).
type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

// keyInjector là một bàn phím ảo dùng để gửi phím tới RetroArch đang chạy. Mỗi
// daemon chỉ cần một instance; đóng fd sẽ hủy thiết bị ảo tương ứng.
type keyInjector struct {
	fd int
	mu sync.Mutex
}

// newKeyInjector tạo bàn phím ảo chỉ với các phím được phép tiêm. Giới hạn danh
// sách phím giúp không bao giờ gửi nhầm tổ hợp hệ thống (ví dụ KEY_POWER).
func newKeyInjector(keys []uint16) (*keyInjector, error) {
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("mở /dev/uinput: %w", err)
	}
	fail := func(err error) (*keyInjector, error) {
		syscall.Close(fd)
		return nil, err
	}
	if err := ioctl(fd, uiSetEvBit, uintptr(evKey)); err != nil {
		return fail(fmt.Errorf("UI_SET_EVBIT: %w", err))
	}
	if err := ioctl(fd, uiSetEvBit, uintptr(evSyn)); err != nil {
		return fail(fmt.Errorf("UI_SET_EVBIT(EV_SYN): %w", err))
	}
	for _, k := range keys {
		if err := ioctl(fd, uiSetKeyBit, uintptr(k)); err != nil {
			return fail(fmt.Errorf("UI_SET_KEYBIT(%d): %w", k, err))
		}
	}
	setup := uinputSetup{BusType: busUSB, Vendor: 0x1, Product: 0x1, Version: 1}
	copy(setup.Name[:], "fn-switch virtual keys")
	if err := ioctl(fd, uiDevSetup, uintptr(unsafe.Pointer(&setup))); err != nil {
		return fail(fmt.Errorf("UI_DEV_SETUP: %w", err))
	}
	if err := ioctl(fd, uiDevCreate, 0); err != nil {
		return fail(fmt.Errorf("UI_DEV_CREATE: %w", err))
	}
	return &keyInjector{fd: fd}, nil
}

// Tap gửi một cú nhấn + nhả kèm SYN_REPORT. Các writes được gom dưới khóa để
// nhiều nút bấm cùng lúc không đan xen sự kiện của nhau.
func (k *keyInjector) Tap(code uint16) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	events := []inputEvent{
		{Type: evKey, Code: code, Value: 1},
		{Type: evKey, Code: code, Value: 0},
		{Type: evSyn, Code: synReport, Value: 0},
	}
	for i := range events {
		data := (*[inputEventSize]byte)(unsafe.Pointer(&events[i]))
		if _, err := syscall.Write(k.fd, data[:]); err != nil {
			return fmt.Errorf("ghi uinput: %w", err)
		}
	}
	return nil
}

// Close hủy thiết bị ảo và giải phóng fd.
func (k *keyInjector) Close() error {
	if k == nil || k.fd < 0 {
		return nil
	}
	_ = ioctl(k.fd, uiDevDestroy, 0)
	err := syscall.Close(k.fd)
	k.fd = -1
	return err
}

// ioctl gọi syscall IOCTL với số argument đúng cho syscall.Syscall trên Linux.
func ioctl(fd int, request, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, arg)
	if errno != 0 {
		return errno
	}
	return nil
}
