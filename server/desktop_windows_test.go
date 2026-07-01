//go:build windows

package main

import (
	"sort"
	"testing"
)

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

func TestDesiredDesktops(t *testing.T) {
	// Unlocked sessions need just the Default desktop; a locked session shows the
	// LockApp curtain on Default AND the credential UI on the Winlogon secure
	// desktop, so both agents must run so iOS can dismiss the curtain (Default)
	// and then type the PIN (Winlogon). Order matters: Default first (spawned as
	// the interactive user, which is cheaper/always available).
	cases := map[int32][]string{
		wtsStateUnlock: {"Default"},
		42:             {"Default"},
		wtsStateLock:   {"Default", "Winlogon"},
	}
	for flags, want := range cases {
		got := desiredDesktops(flags)
		if len(got) != len(want) {
			t.Fatalf("desiredDesktops(%d) = %v, want %v", flags, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("desiredDesktops(%d) = %v, want %v", flags, got, want)
			}
		}
	}
}

func TestPlanAgents(t *testing.T) {
	cases := []struct {
		name      string
		alive     map[string]bool
		want      []string
		wantStop  []string
		wantStart []string
	}{
		{"cold start locked", nil, []string{"Default", "Winlogon"}, nil, []string{"Default", "Winlogon"}},
		{"steady unlocked", map[string]bool{"Default": true}, []string{"Default"}, nil, nil},
		{"lock: add winlogon", map[string]bool{"Default": true}, []string{"Default", "Winlogon"}, nil, []string{"Winlogon"}},
		{"unlock: drop winlogon", map[string]bool{"Default": true, "Winlogon": true}, []string{"Default"}, []string{"Winlogon"}, nil},
		{"start order follows want", map[string]bool{}, []string{"Default", "Winlogon"}, nil, []string{"Default", "Winlogon"}},
	}
	for _, c := range cases {
		stop, start := planAgents(c.alive, c.want)
		sort.Strings(stop)
		if !eqStrs(stop, c.wantStop) || !eqStrs(start, c.wantStart) {
			t.Errorf("%s: planAgents(%v,%v) = stop%v start%v; want stop%v start%v",
				c.name, c.alive, c.want, stop, start, c.wantStop, c.wantStart)
		}
	}
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
