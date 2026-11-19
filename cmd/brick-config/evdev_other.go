//go:build !linux

package main

import "sync/atomic"

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

func StartEvdevListener() {
	// Giả lập trên Windows / macOS: không làm gì
}
