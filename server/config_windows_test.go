//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestConfigRoundTripUnderHKCU(t *testing.T) {
	// Use a throwaway HKCU key so the test needs no admin rights.
	root := registry.CURRENT_USER
	defer registry.DeleteKey(root, configPath)

	want := serverConfig{Password: "s3cret", Port: 28000, Name: "客厅PC"}
	if err := writeConfig(root, want); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	got, err := readConfig(root)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadConfigMissingKeyReturnsDefaults(t *testing.T) {
	registry.DeleteKey(registry.CURRENT_USER, configPath) // ensure absent
	got, err := readConfig(registry.CURRENT_USER)
	if err == nil {
		t.Fatalf("expected error for missing key")
	}
	if got.Port != 27500 || got.Password != "1234" {
		t.Fatalf("defaults not returned: %+v", got)
	}
}
