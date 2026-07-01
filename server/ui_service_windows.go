//go:build windows

package main

import (
	"log"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"
)

// serveUI is the default (no-flag) mode. When the RemoteMouse service is
// installed and running it owns the TCP port and mDNS, so this acts as a
// controller UI only (status from HKLM), avoiding a fight for port 27500.
// Otherwise it serves directly — announce + listen + inject on the unlocked
// desktop — restoring the simple single-process mode so the phone can discover
// it without installing the service.
func serveUI(cfg appConfig) {
	if serviceRunning() {
		if sc, err := readConfig(registry.LOCAL_MACHINE); err == nil {
			cfg.pass, cfg.port = sc.Password, sc.Port
		}
		ips := DetectIPs()
		reg := NewClientRegistry()
		runUI(cfg.notray, cfg.pass, cfg.port, ips, reg)
		return
	}
	serveStandalone(cfg)
}

// elevatedArgLine builds a Windows command line from args, quoting/escaping each
// one so values containing spaces (e.g. a device name or password) survive the
// child process's flag parsing. syscall.EscapeArg applies the CommandLineToArgvW
// quoting rules and leaves space-free tokens untouched.
func elevatedArgLine(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = syscall.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}

// elevatedSelf relaunches this executable elevated (UAC) with args, used for
// install/uninstall which require admin.
func elevatedSelf(args ...string) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(selfPath())
	argp, _ := windows.UTF16PtrFromString(elevatedArgLine(args))
	if err := windows.ShellExecute(0, verb, file, argp, nil, windows.SW_NORMAL); err != nil {
		log.Printf("elevatedSelf %v: %v", args, err)
	}
}

// serviceRunning reports whether the RemoteMouse service is installed and in the
// RUNNING state, using read-only SCM access (no admin required). Used by the
// default mode to decide whether to serve directly or act as a controller.
func serviceRunning() bool {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString(svcName)
	sh, err := windows.OpenService(scm, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(sh)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(sh, &st); err != nil {
		return false
	}
	return st.CurrentState == windows.SERVICE_RUNNING
}

// serviceState returns a human-readable service status using read-only SCM
// access (no admin required).
func serviceState() string {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return "未知"
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString(svcName)
	sh, err := windows.OpenService(scm, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return "未安装"
	}
	defer windows.CloseServiceHandle(sh)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(sh, &st); err != nil {
		return "未知"
	}
	switch st.CurrentState {
	case windows.SERVICE_RUNNING:
		return "运行中"
	case windows.SERVICE_STOPPED:
		return "已停止"
	case windows.SERVICE_START_PENDING:
		return "启动中"
	case windows.SERVICE_STOP_PENDING:
		return "停止中"
	default:
		return "未知"
	}
}

// serviceStartType returns the service start type using read-only SCM access
// (no admin). ok is false when the service is not installed or unreadable. It
// wraps a self-opened handle in mgr.Service to reuse mgr's config parsing while
// avoiding mgr.Connect (which demands admin).
func serviceStartType() (uint32, bool) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, false
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString(svcName)
	h, err := windows.OpenService(scm, name, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return 0, false // not installed
	}
	defer windows.CloseServiceHandle(h)
	s := &mgr.Service{Name: svcName, Handle: h}
	cfg, err := s.Config()
	if err != nil {
		return 0, false
	}
	return cfg.StartType, true
}

// serviceInstalled reports whether the service exists (read-only).
func serviceInstalled() bool {
	_, ok := serviceStartType()
	return ok
}
