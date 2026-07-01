//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestStartTypeFromArg(t *testing.T) {
	cases := map[string]struct {
		val uint32
		ok  bool
	}{
		"auto":     {windows.SERVICE_AUTO_START, true},
		"manual":   {windows.SERVICE_DEMAND_START, true},
		"disabled": {windows.SERVICE_DISABLED, true},
		"bogus":    {0, false},
	}
	for arg, want := range cases {
		got, ok := startTypeFromArg(arg)
		if got != want.val || ok != want.ok {
			t.Errorf("startTypeFromArg(%q) = (%d,%v), want (%d,%v)", arg, got, ok, want.val, want.ok)
		}
	}
}

func TestStartTypeArgRoundTrip(t *testing.T) {
	for _, arg := range []string{"auto", "manual", "disabled"} {
		v, _ := startTypeFromArg(arg)
		if back := startTypeArg(v); back != arg {
			t.Errorf("startTypeArg(fromArg(%q)) = %q", arg, back)
		}
	}
}

func TestStartTypeIndexRoundTrip(t *testing.T) {
	for i := 0; i < 3; i++ {
		if got := startTypeIndex(startTypeByIndex(i)); got != i {
			t.Errorf("index round-trip i=%d got %d", i, got)
		}
	}
}

func TestStartTypeLabel(t *testing.T) {
	if startTypeLabel(windows.SERVICE_AUTO_START) != "自动" {
		t.Errorf("auto label wrong")
	}
	if startTypeLabel(9999) != "未知" {
		t.Errorf("unknown label wrong")
	}
}
