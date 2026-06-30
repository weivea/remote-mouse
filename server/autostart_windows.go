//go:build windows

package main

import (
	"os"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runVal = "RemoteMouse"

func autostartCmd(pass string, port int) string {
	exe, _ := os.Executable()
	return `"` + exe + `" -pass ` + pass + " -port " + strconv.Itoa(port)
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
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if on {
		k.SetStringValue(runVal, autostartCmd(pass, port))
	} else {
		k.DeleteValue(runVal)
	}
}
