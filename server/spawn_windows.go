//go:build windows

package main

import (
	"fmt"
	"log"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procProcessIdToSessId = modKernel32.NewProc("ProcessIdToSessionId")
)

// enablePrivilege enables a named privilege on the current process token.
func enablePrivilege(name string) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()

	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(name), &luid); err != nil {
		return fmt.Errorf("LookupPrivilegeValue %s: %w", name, err)
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}

// enableServicePrivileges enables the privileges LocalSystem needs to query a
// user token and launch a process as that user on another desktop.
func enableServicePrivileges() {
	for _, p := range []string{"SeTcbPrivilege", "SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege"} {
		if err := enablePrivilege(p); err != nil {
			log.Printf("enablePrivilege %s: %v", p, err)
		}
	}
}

// userTokenForSession returns a primary token for the interactive user in the
// session (used when the input desktop is Default / unlocked).
func userTokenForSession(session uint32) (windows.Token, error) {
	var tok windows.Token
	if err := windows.WTSQueryUserToken(session, &tok); err != nil {
		return 0, fmt.Errorf("WTSQueryUserToken(%d): %w", session, err)
	}
	defer tok.Close()
	return duplicatePrimary(tok)
}

// winlogonTokenForSession returns a primary token cloned from winlogon.exe in
// the session (a SYSTEM token that can reach the secure desktop).
func winlogonTokenForSession(session uint32) (windows.Token, error) {
	pid, err := findProcessInSession("winlogon.exe", session)
	if err != nil {
		return 0, err
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, pid)
	if err != nil {
		return 0, fmt.Errorf("OpenProcess winlogon pid=%d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	var tok windows.Token
	if err := windows.OpenProcessToken(h,
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY, &tok); err != nil {
		return 0, fmt.Errorf("OpenProcessToken winlogon: %w", err)
	}
	defer tok.Close()
	return duplicatePrimary(tok)
}

func duplicatePrimary(tok windows.Token) (windows.Token, error) {
	var dup windows.Token
	if err := windows.DuplicateTokenEx(tok, windows.MAXIMUM_ALLOWED, nil,
		windows.SecurityImpersonation, windows.TokenPrimary, &dup); err != nil {
		return 0, fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	return dup, nil
}

// findProcessInSession returns the PID of the first process named exe (case-
// insensitive) running in session, via a Toolhelp snapshot + ProcessIdToSessionId.
func findProcessInSession(exe string, session uint32) (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), exe) {
			continue
		}
		var sess uint32
		r, _, _ := procProcessIdToSessId.Call(uintptr(pe.ProcessID), uintptr(unsafe.Pointer(&sess)))
		if r != 0 && sess == session {
			return pe.ProcessID, nil
		}
	}
	return 0, fmt.Errorf("%s not found in session %d", exe, session)
}

// spawnAgentOnDesktop launches exePath as the given token, bound to
// WinSta0\<desktop>, telling it to connect to the inject pipe. When absolute is
// set (secure desktop) the agent is told to use absolute mouse positioning,
// since the secure desktop ignores relative moves. It returns the process handle
// so the caller can TerminateProcess it on the next desktop switch or shutdown.
func spawnAgentOnDesktop(token windows.Token, exePath, desktop, pipe string, absolute, injectLog bool) (windows.Handle, error) {
	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, token, false); err != nil {
		return 0, fmt.Errorf("CreateEnvironmentBlock: %w", err)
	}
	defer windows.DestroyEnvironmentBlock(env)

	deskPath := `WinSta0\` + desktop
	cmd := fmt.Sprintf(`"%s" -agent -pipe %s`, exePath, pipe)
	if absolute {
		cmd += " -absolute"
	}
	if injectLog {
		cmd += " -injectlog"
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = windows.StringToUTF16Ptr(deskPath)

	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(token, nil, windows.StringToUTF16Ptr(cmd),
		nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW,
		env, nil, &si, &pi); err != nil {
		return 0, fmt.Errorf("CreateProcessAsUser desktop=%s: %w", deskPath, err)
	}
	windows.CloseHandle(pi.Thread)
	return pi.Process, nil
}
