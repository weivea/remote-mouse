//go:build windows

package main

import "testing"

func TestElevatedArgLine(t *testing.T) {
	got := elevatedArgLine([]string{"-apply-config", "-name", "Living Room", "-pass", "p q", "-port", "27500"})
	want := `-apply-config -name "Living Room" -pass "p q" -port 27500`
	if got != want {
		t.Fatalf("elevatedArgLine = %q, want %q", got, want)
	}
}

func TestElevatedArgLineEdgeCases(t *testing.T) {
	// An empty arg must become "" so it survives as a distinct empty token, and
	// an embedded double-quote must be backslash-escaped (CommandLineToArgvW
	// rules) so the elevated child parses the flag value intact.
	got := elevatedArgLine([]string{"-name", "", "-pass", `p"q`})
	want := `-name "" -pass p\"q`
	if got != want {
		t.Fatalf("elevatedArgLine = %q, want %q", got, want)
	}
}
