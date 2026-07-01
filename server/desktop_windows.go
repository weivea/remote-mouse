//go:build windows

package main

import (
	"fmt"
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

var (
	modWtsapi32              = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySessionInfoW = modWtsapi32.NewProc("WTSQuerySessionInformationW")
)

const (
	wtsSessionInfoEx        = 25         // WTS_INFO_CLASS: WTSSessionInfoEx
	wtsStateLock     int32  = 0          // WTS_SESSIONSTATE_LOCK   (Windows 8+; reversed on Win7)
	wtsStateUnlock   int32  = 1          // WTS_SESSIONSTATE_UNLOCK
	wtsNoSession     uint32 = 0xFFFFFFFF // WTSGetActiveConsoleSessionId: no session attached
)

// currentInputDesktop returns the name of the desktop currently receiving user
// input in the CALLER'S session: "Default" when unlocked, "Winlogon" on the
// lock/login secure desktop, or "Screen-saver". It only works from an
// interactive session (the -probe-desktop dev mode); a session-0 service is on
// window station Service-0x0-3e7$ and cannot observe the interactive input
// desktop this way, so the service uses sessionLockState/desiredDesktops instead.
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

// sessionLockState returns the raw WTSINFOEX.SessionFlags for a session.
// WTSQuerySessionInformation works from the session-0 service (unlike
// OpenInputDesktop), so this is how the service learns whether the console
// session is on the secure/lock desktop.
func sessionLockState(sess uint32) (flags int32, err error) {
	if sess == wtsNoSession {
		return 0, fmt.Errorf("no active console session")
	}
	var buf unsafe.Pointer
	var n uint32
	r, _, e := procWTSQuerySessionInfoW.Call(0, uintptr(sess), wtsSessionInfoEx,
		uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return 0, e
	}
	defer windows.WTSFreeMemory(uintptr(buf))
	// WTSINFOEXW (amd64): DWORD Level @0, 4-byte pad, WTSINFOEX_LEVEL1_W @8 whose
	// LONG SessionFlags sits at offset 16.
	return *(*int32)(unsafe.Add(buf, 16)), nil
}

// desktopForFlags maps WTSINFOEX.SessionFlags to the desktop the service must
// target for injection: a locked session is on the "Winlogon" secure desktop,
// anything else is the ordinary "Default" desktop.
func desktopForFlags(flags int32) string {
	if flags == wtsStateLock {
		return "Winlogon"
	}
	return "Default"
}

// desiredDesktops returns the set of desktops the service must run an agent on
// for the given session lock flags. Unlocked: just Default. Locked: BOTH the
// Default desktop (the LockApp "curtain" — wallpaper/clock — lives here and is
// dismissed by a user-token mouse move+click) AND the Winlogon secure desktop
// (the LogonUI credential/PIN box lives here and only accepts SYSTEM-token
// Unicode text). Running both and broadcasting every event lets whichever
// desktop is the active input desktop accept it while the other harmlessly
// rejects with ACCESS_DENIED, so iOS can drive the whole unlock: move+click to
// dismiss the curtain, then type the PIN. Order matters: Default first.
func desiredDesktops(flags int32) []string {
	primary := desktopForFlags(flags) // Winlogon when locked, else Default
	if flags == wtsStateLock {
		return []string{"Default", primary}
	}
	return []string{primary}
}

// planAgents diffs the currently-alive agent desktops against the desired set,
// returning which to stop (alive but no longer wanted) and which to start
// (wanted but not alive). start follows want's order so the Default/curtain
// agent is spawned before the Winlogon/credential agent.
func planAgents(alive map[string]bool, want []string) (stop, start []string) {
	wantSet := make(map[string]bool, len(want))
	for _, d := range want {
		wantSet[d] = true
	}
	for d := range alive {
		if !wantSet[d] {
			stop = append(stop, d)
		}
	}
	for _, d := range want {
		if !alive[d] {
			start = append(start, d)
		}
	}
	return stop, start
}

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
