//go:build windows

package main

import "golang.org/x/sys/windows"

// startTypeFromArg maps a CLI arg ("auto"/"manual"/"disabled") to the Win32
// service start type. ok is false for unrecognized input.
func startTypeFromArg(s string) (uint32, bool) {
	switch s {
	case "auto":
		return windows.SERVICE_AUTO_START, true
	case "manual":
		return windows.SERVICE_DEMAND_START, true
	case "disabled":
		return windows.SERVICE_DISABLED, true
	default:
		return 0, false
	}
}

// startTypeArg is the inverse of startTypeFromArg; returns "" for unknown.
func startTypeArg(t uint32) string {
	switch t {
	case windows.SERVICE_AUTO_START:
		return "auto"
	case windows.SERVICE_DEMAND_START:
		return "manual"
	case windows.SERVICE_DISABLED:
		return "disabled"
	default:
		return ""
	}
}

// startTypeLabel is the Chinese label shown in the combo box.
func startTypeLabel(t uint32) string {
	switch t {
	case windows.SERVICE_AUTO_START:
		return "自动"
	case windows.SERVICE_DEMAND_START:
		return "手动"
	case windows.SERVICE_DISABLED:
		return "禁用"
	default:
		return "未知"
	}
}

// startTypeCombo is the fixed combo-box order.
var startTypeCombo = []uint32{
	windows.SERVICE_AUTO_START,
	windows.SERVICE_DEMAND_START,
	windows.SERVICE_DISABLED,
}

// startTypeIndex returns the combo-box row for a start type, or -1.
func startTypeIndex(t uint32) int {
	for i, v := range startTypeCombo {
		if v == t {
			return i
		}
	}
	return -1
}

// startTypeByIndex returns the start type for a combo-box row; out-of-range
// falls back to auto.
func startTypeByIndex(i int) uint32 {
	if i < 0 || i >= len(startTypeCombo) {
		return windows.SERVICE_AUTO_START
	}
	return startTypeCombo[i]
}

// startTypeLabels returns the combo-box display strings in order.
func startTypeLabels() []string {
	out := make([]string, len(startTypeCombo))
	for i, v := range startTypeCombo {
		out[i] = startTypeLabel(v)
	}
	return out
}
