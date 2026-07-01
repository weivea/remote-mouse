//go:build windows

package main

import "testing"

func TestTokenStrategyFor(t *testing.T) {
	cases := map[string]tokenStrategy{
		"Default":      tokenUser,
		"Winlogon":     tokenWinlogon,
		"Screen-saver": tokenWinlogon,
		"whatever":     tokenWinlogon,
	}
	for desk, want := range cases {
		if got := tokenStrategyFor(desk); got != want {
			t.Errorf("tokenStrategyFor(%q) = %v, want %v", desk, got, want)
		}
	}
}

func TestDesktopForFlags(t *testing.T) {
	cases := map[int32]string{
		wtsStateLock:   "Winlogon",
		wtsStateUnlock: "Default",
		42:             "Default", // unknown/other -> treat as unlocked
	}
	for flags, want := range cases {
		if got := desktopForFlags(flags); got != want {
			t.Errorf("desktopForFlags(%d) = %q, want %q", flags, got, want)
		}
	}
}
