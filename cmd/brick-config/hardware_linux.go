//go:build linux

package main

import (
	"syscall"
	"unsafe"
)

// queryFNSwitchState đọc trạng thái switch từ /dev/input/event3 bằng ioctl EVIOCGSW trên Linux
func queryFNSwitchState() bool {
	fd, err := syscall.Open("/dev/input/event3", syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(fd)

	var swMask uint32
	// EVIOCGSW(4) = _IOC(_IOC_READ, 'E', 0x1b, 4) = 0x8004451b
	const EVIOCGSW4 = 0x8004451b
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(EVIOCGSW4), uintptr(unsafe.Pointer(&swMask)))
	if errno != 0 {
		return false
	}
	// Code 1 = SW_TABLET_MODE (bit 1)
	return (swMask & (1 << 1)) != 0
}
