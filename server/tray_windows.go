//go:build windows

package main

import (
	_ "embed"
	"log"
	"os"
	"strconv"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows/registry"
)

//go:embed assets/tray.ico
var trayIcon []byte

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

// runUI blocks on the tray loop (or forever when notray). Listening runs in a goroutine.
func runUI(notray bool, pass string, port int) {
	if notray {
		select {}
	}
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTitle("RemoteMouse")
		systray.SetTooltip("RemoteMouse server")
		systray.AddMenuItem("password "+pass+"  port "+strconv.Itoa(port), "").Disable()
		mAuto := systray.AddMenuItemCheckbox("Start at login", "", autostartOn())
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "")
		go func() {
			for {
				select {
				case <-mAuto.ClickedCh:
					if mAuto.Checked() {
						setAutostart(false, pass, port)
						mAuto.Uncheck()
					} else {
						setAutostart(true, pass, port)
						mAuto.Check()
					}
				case <-mQuit.ClickedCh:
					systray.Quit()
				}
			}
		}()
		log.Print("tray ready")
	}, func() { os.Exit(0) })
}
