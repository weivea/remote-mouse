//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestAutostartCmdEscapesArgs(t *testing.T) {
	cmd := autostartCmd("p q", 27500, "Living Room")
	if !strings.Contains(cmd, `-pass "p q"`) {
		t.Errorf("password with space must be quoted: %q", cmd)
	}
	if !strings.Contains(cmd, `-name "Living Room"`) {
		t.Errorf("name with space must be quoted: %q", cmd)
	}
	if !strings.Contains(cmd, "-port 27500") {
		t.Errorf("port missing/altered: %q", cmd)
	}
}
