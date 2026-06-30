//go:build windows

package main

import (
	"log"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// serveUI is the default (no-flag) mode: a tray UI that shows connection info
// from HKLM and installs/controls the service. It does NOT listen on TCP or
// inject — the service owns that. The device list is empty here (the service
// owns the real connections); a status pipe to populate it is a POC follow-up.
func serveUI(cfg appConfig) {
	if sc, err := readConfig(registry.LOCAL_MACHINE); err == nil {
		cfg.pass, cfg.port = sc.Password, sc.Port
	}
	ips := DetectIPs()
	reg := NewClientRegistry()
	runUI(cfg.notray, cfg.pass, cfg.port, ips, reg)
}

// elevatedSelf relaunches this executable elevated (UAC) with args, used for
// install/uninstall which require admin.
func elevatedSelf(args ...string) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(selfPath())
	argp, _ := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err := windows.ShellExecute(0, verb, file, argp, nil, windows.SW_NORMAL); err != nil {
		log.Printf("elevatedSelf %v: %v", args, err)
	}
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
