package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// HardwareInfo chứa thông tin phần cứng theo thời gian thực
type HardwareInfo struct {
	BatteryPercent int
	IsCharging     bool
	CPUTemp        float64
	CPUFreqMHz     int
	Brightness     int
	Volume         int
	FNSwitchOn     bool
}

// ReadHardwareInfo quét thông tin phần cứng trên TrimUI Brick Pro / Knulli
func ReadHardwareInfo() HardwareInfo {
	info := HardwareInfo{
		BatteryPercent: 100,
		IsCharging:     false,
		CPUTemp:        45.0,
		CPUFreqMHz:     1800,
		Brightness:     100,
		Volume:         80,
		FNSwitchOn:     false,
	}

	// 1. Pin (Battery)
	for _, path := range []string{
		"/sys/class/power_supply/axp2202-battery/capacity",
		"/sys/class/power_supply/battery/capacity",
	} {
		if val, err := readIntFromFile(path); err == nil {
			info.BatteryPercent = val
			break
		}
	}

	// Trạng thái sạc
	for _, path := range []string{
		"/sys/class/power_supply/axp2202-battery/status",
		"/sys/class/power_supply/battery/status",
	} {
		if data, err := os.ReadFile(path); err == nil {
			status := strings.TrimSpace(string(data))
			if strings.EqualFold(status, "Charging") {
				info.IsCharging = true
			}
			break
		}
	}

	// 2. Nhiệt độ CPU
	if tempRaw, err := readIntFromFile("/sys/class/thermal/thermal_zone0/temp"); err == nil {
		if tempRaw > 1000 {
			info.CPUTemp = float64(tempRaw) / 1000.0
		} else {
			info.CPUTemp = float64(tempRaw)
		}
	}

	// 3. Xung nhịp CPU hiện tại
	if freqRaw, err := readIntFromFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq"); err == nil {
		info.CPUFreqMHz = freqRaw / 1000
	}

	// 4. Độ sáng màn hình
	if out, err := exec.Command("/usr/bin/knulli-brightness").Output(); err == nil {
		if val, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			info.Brightness = val
		}
	} else if bRaw, err := readIntFromFile("/sys/class/backlight/backlight/brightness"); err == nil {
		info.Brightness = (bRaw * 100) / 255
	}

	// 5. Âm lượng hệ thống
	if out, err := exec.Command("/usr/bin/knulli-audio", "getSystemVolume").Output(); err == nil {
		if val, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			info.Volume = val
		}
	}

	// 6. Trạng thái công tắc FN
	info.FNSwitchOn = queryFNSwitchState()

	return info
}

func readIntFromFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// LivePreviewLEDs áp dụng thử nghiệm thông số LED ngay lập tức lên phần cứng
func LivePreviewLEDs(topScale, lrScale, rearScale, f1f2Scale int, breath bool) {
	_ = os.WriteFile("/sys/class/led_anim/max_scale", []byte(fmt.Sprintf("%d", topScale)), 0644)
	_ = os.WriteFile("/sys/class/led_anim/max_scale_lr", []byte(fmt.Sprintf("%d", lrScale)), 0644)
	_ = os.WriteFile("/sys/class/led_anim/max_scale_rear", []byte(fmt.Sprintf("%d", rearScale)), 0644)
	_ = os.WriteFile("/sys/class/led_anim/max_scale_f1f2", []byte(fmt.Sprintf("%d", f1f2Scale)), 0644)

	if breath {
		_ = os.WriteFile("/sys/class/led_anim/effect_enable", []byte("1"), 0644)
		for _, node := range []string{"effect_lr", "effect_m", "effect_rear", "effect_f1", "effect_f2"} {
			_ = os.WriteFile(filepath.Join("/sys/class/led_anim", node), []byte("2"), 0644)
		}
		for _, node := range []string{"effect_cycles_lr", "effect_cycles_m", "effect_cycles_rear", "effect_cycles_f1", "effect_cycles_f2"} {
			_ = os.WriteFile(filepath.Join("/sys/class/led_anim", node), []byte("-1"), 0644)
		}
	}
}
