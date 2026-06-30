//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestFindProcessInSessionFindsSelf(t *testing.T) {
	// The running test binary must be findable in its own session.
	var sess uint32
	r, _, _ := procProcessIdToSessId.Call(
		uintptr(windows.GetCurrentProcessId()), uintptr(unsafe.Pointer(&sess)))
	if r == 0 {
		t.Skip("ProcessIdToSessionId unavailable")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	name := filepath.Base(exe)
	pid, err := findProcessInSession(name, sess)
	if err != nil {
		t.Fatalf("findProcessInSession(%q, %d): %v", name, sess, err)
	}
	if pid == 0 {
		t.Fatalf("pid = 0")
	}
}
