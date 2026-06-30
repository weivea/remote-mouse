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
