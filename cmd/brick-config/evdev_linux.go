//go:build linux

package main

import (
	"encoding/binary"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

type EvdevState struct {
	F1Pressed   atomic.Bool
	F2Pressed   atomic.Bool
	HomePressed atomic.Bool
	L1Pressed   atomic.Bool
	R1Pressed   atomic.Bool
	L2Pressed   atomic.Bool
	R2Pressed   atomic.Bool
	FNSwitch    atomic.Bool
}

var globalEvdev EvdevState

// StartEvdevListener mở kết nối tới /dev/input/event3 và event0 để đọc các phím phụ
func StartEvdevListener() {
	queryInitialFNSwitch()

	// Lắng nghe controller chính (/dev/input/event3)
	go listenDeviceEvents("/dev/input/event3")

	// Lắng nghe bàn phím hệ thống (/dev/input/event0) dự phòng cho phím Home/Power
	go listenDeviceEvents("/dev/input/event0")
}

func queryInitialFNSwitch() {
	fd, err := syscall.Open("/dev/input/event3", syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer syscall.Close(fd)

	var swMask uint32
	const EVIOCGSW4 = 0x8004451b
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(EVIOCGSW4), uintptr(unsafe.Pointer(&swMask)))
	if errno == 0 {
		globalEvdev.FNSwitch.Store((swMask & (1 << 1)) != 0)
	}
}

func listenDeviceEvents(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	var buf [24]byte
	for {
		if _, err := io.ReadFull(f, buf[:]); err != nil {
			return
		}

		evType := binary.LittleEndian.Uint16(buf[16:18])
		evCode := binary.LittleEndian.Uint16(buf[18:20])
		evVal := int32(binary.LittleEndian.Uint32(buf[20:24]))
		pressed := (evVal != 0)

		if evType == 0x01 { // EV_KEY
			switch evCode {
			case 59: // KEY_F1
				globalEvdev.F1Pressed.Store(pressed)
			case 60: // KEY_F2
				globalEvdev.F2Pressed.Store(pressed)
			case 172, 139, 102: // KEY_HOMEPAGE (172), KEY_MENU (139), KEY_HOME (102)
				globalEvdev.HomePressed.Store(pressed)
			case 310, 308, 104: // BTN_TL, BTN_SIDE, KEY_PAGEUP -> L1
				globalEvdev.L1Pressed.Store(pressed)
			case 311, 309, 109: // BTN_TR, BTN_EXTRA, KEY_PAGEDOWN -> R1
				globalEvdev.R1Pressed.Store(pressed)
			case 312: // BTN_TL2 -> L2
				globalEvdev.L2Pressed.Store(pressed)
			case 313: // BTN_TR2 -> R2
				globalEvdev.R2Pressed.Store(pressed)
			}
		} else if evType == 0x03 { // EV_ABS (Cò analog trigger)
			triggerPressed := (evVal > 120)
			switch evCode {
			case 2, 10: // ABS_Z, ABS_BRAKE -> L2
				globalEvdev.L2Pressed.Store(triggerPressed)
			case 5, 9: // ABS_RZ, ABS_GAS -> R2
				globalEvdev.R2Pressed.Store(triggerPressed)
			}
		} else if evType == 0x05 { // EV_SW
			if evCode == 1 { // SW_TABLET_MODE (Công tắc FN)
				globalEvdev.FNSwitch.Store(pressed)
			}
		}
	}
}
