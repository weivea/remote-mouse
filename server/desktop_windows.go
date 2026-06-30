//go:build windows

package main

import (
	"log"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32                    = windows.NewLazySystemDLL("user32.dll")
	procOpenInputDesktop         = modUser32.NewProc("OpenInputDesktop")
	procGetUserObjectInformation = modUser32.NewProc("GetUserObjectInformationW")
	procCloseDesktop             = modUser32.NewProc("CloseDesktop")
)

const uoiName = 2 // UOI_NAME

// currentInputDesktop returns the name of the desktop currently receiving user
// input: "Default" when unlocked, "Winlogon" on the lock/login secure desktop,
// or "Screen-saver". Reading the name needs no special privilege, but a normal
// user-session process may be denied OpenInputDesktop while a secure desktop is
// active — the SYSTEM service is the authoritative caller.
func currentInputDesktop() (string, error) {
	hd, _, err := procOpenInputDesktop.Call(0, 0, uintptr(windows.GENERIC_READ))
	if hd == 0 {
		return "", err
	}
	defer procCloseDesktop.Call(hd)

	var buf [256]uint16
	var needed uint32
	r, _, err := procGetUserObjectInformation.Call(
		hd, uintptr(uoiName),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)*2),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return "", err
	}
	return syscall.UTF16ToString(buf[:]), nil
}

// activeConsoleSession returns the session attached to the physical console.
func activeConsoleSession() uint32 { return windows.WTSGetActiveConsoleSessionId() }

type tokenStrategy int

const (
	tokenUser     tokenStrategy = iota // interactive user token (Default desktop)
	tokenWinlogon                      // winlogon SYSTEM token (secure desktop)
)

// tokenStrategyFor maps an input-desktop name to the token strategy the service
// must use to spawn an agent that can inject on that desktop. Anything that is
// not the unlocked "Default" desktop is treated as secure.
func tokenStrategyFor(desktop string) tokenStrategy {
	if desktop == "Default" {
		return tokenUser
	}
	return tokenWinlogon
}

// probeDesktop prints the current input desktop in a loop (the -probe-desktop
// dev mode). Run it, then Win+L and unlock, and read the console scrollback to
// confirm the Default<->Winlogon transitions.
func probeDesktop() {
	for {
		if name, err := currentInputDesktop(); err != nil {
			log.Printf("probe: OpenInputDesktop failed: %v (likely a secure desktop we can't read)", err)
		} else {
			log.Printf("probe: input desktop=%q console session=%d", name, activeConsoleSession())
		}
		time.Sleep(500 * time.Millisecond)
	}
}
