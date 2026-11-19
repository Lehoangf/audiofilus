//go:build !linux

package main

// queryFNSwitchState giả lập trên các OS khác
func queryFNSwitchState() bool {
	return false
}
