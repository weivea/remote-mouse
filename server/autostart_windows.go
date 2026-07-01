//go:build windows

package main

import (
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runVal = "RemoteMouse"

func autostartCmd(pass string, port int, name string) string {
	exe, _ := os.Executable()
	cmd := syscall.EscapeArg(exe) + " -pass " + syscall.EscapeArg(pass) + " -port " + strconv.Itoa(port)
	if strings.TrimSpace(name) != "" {
		cmd += " -name " + syscall.EscapeArg(name)
	}
	return cmd
}

func autostartOn() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runVal)
	return err == nil
}

func setAutostart(on bool, pass string, port int) {
	setAutostartNamed(on, pass, port, "")
}

func setAutostartNamed(on bool, pass string, port int, name string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if on {
		k.SetStringValue(runVal, autostartCmd(pass, port, name))
	} else {
		k.DeleteValue(runVal)
	}
}
