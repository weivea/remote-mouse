//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

//go:embed assets/tray.ico
var trayICO []byte

// deviceModel feeds the connected-devices TableView from a ClientRegistry.
type deviceModel struct {
	walk.TableModelBase
	reg   *ClientRegistry
	items []Client
}

func (m *deviceModel) RowCount() int { return len(m.items) }

func (m *deviceModel) Value(row, col int) interface{} {
	c := m.items[row]
	switch col {
	case 0:
		if c.Name == "" {
			return "(未命名)"
		}
		return c.Name
	case 1:
		return c.Addr
	case 2:
		return fmtDur(time.Since(c.Since))
	default:
		return "在线"
	}
}

func (m *deviceModel) reload() {
	m.items = m.reg.Snapshot()
	m.PublishRowsReset()
}

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
}

// loadIcon writes the embedded .ico to a temp file and loads it as a walk.Icon.
func loadIcon() *walk.Icon {
	tmp := filepath.Join(os.TempDir(), "rmserver_tray.ico")
	if err := os.WriteFile(tmp, trayICO, 0o644); err != nil {
		return nil
	}
	ic, err := walk.NewIconFromFile(tmp)
	if err != nil {
		return nil
	}
	return ic
}

// showWindow restores and brings the main window to the foreground.
func showWindow(mw *walk.MainWindow) {
	mw.SetVisible(true)
	win.ShowWindow(mw.Handle(), win.SW_RESTORE)
	win.SetForegroundWindow(mw.Handle())
}

// copyClip writes text to the clipboard, logging (not crashing) on failure.
func copyClip(s string) {
	if err := walk.Clipboard().SetText(s); err != nil {
		log.Printf("clipboard copy failed: %v", err)
	}
}

func runUI(notray bool, pass string, port int, ips HostIPs, reg *ClientRegistry) {
	if notray {
		select {} // headless background, same as before
	}
	runtime.LockOSThread() // walk GUI must own its OS thread

	primary := ips.Primary
	if primary == "" {
		primary = "未检测到局域网 IP"
	}
	secText := strings.Join(ips.Secondary, "   ")

	var mw *walk.MainWindow
	var devCount *walk.Label
	var secLabel *walk.Label
	model := &deviceModel{reg: reg}

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "RemoteMouse",
		MinSize:  Size{Width: 440, Height: 380},
		Size:     Size{Width: 460, Height: 420},
		Layout:   VBox{},
		Children: []Widget{
			GroupBox{
				Title:  "连接信息",
				Layout: Grid{Columns: 3},
				Children: []Widget{
					Label{Text: "推荐 IP"},
					Label{Text: primary, Font: Font{Bold: true, PointSize: 11}},
					PushButton{Text: "复制", OnClicked: func() { copyClip(ips.Primary) }},

					Label{Text: "端口"},
					Label{Text: strconv.Itoa(port)},
					PushButton{Text: "复制", OnClicked: func() { copyClip(strconv.Itoa(port)) }},

					Label{Text: "密码"},
					Label{Text: pass},
					PushButton{Text: "复制", OnClicked: func() { copyClip(pass) }},

					Label{Text: "其他地址"},
					Label{AssignTo: &secLabel, Text: secText, Visible: false},
					PushButton{
						Text:    "展开",
						Enabled: secText != "",
						OnClicked: func() {
							secLabel.SetVisible(!secLabel.Visible())
						},
					},
				},
			},
			Label{AssignTo: &devCount, Text: "已连接设备 (0)"},
			TableView{
				MinSize: Size{Height: 170},
				Columns: []TableViewColumn{
					{Title: "设备名", Width: 130},
					{Title: "IP", Width: 130},
					{Title: "时长", Width: 80},
					{Title: "状态", Width: 60},
				},
				Model: model,
			},
		},
	}).Create(); err != nil {
		log.Printf("walk UI init failed, running headless: %v", err)
		select {}
	}

	// Tray icon + context menu.
	if ni, err := walk.NewNotifyIcon(mw); err == nil {
		defer ni.Dispose()
		if ic := loadIcon(); ic != nil {
			ni.SetIcon(ic)
			mw.SetIcon(ic)
		}
		ni.SetToolTip("RemoteMouse  " + primary + ":" + strconv.Itoa(port))
		ni.SetVisible(true)

		// Left click toggles window visibility.
		ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
			if button != walk.LeftButton {
				return
			}
			if mw.Visible() {
				mw.SetVisible(false)
			} else {
				showWindow(mw)
			}
		})

		showAct := walk.NewAction()
		showAct.SetText("显示窗口")
		showAct.Triggered().Attach(func() { showWindow(mw) })
		ni.ContextMenu().Actions().Add(showAct)

		autoAct := walk.NewAction()
		autoAct.SetText("开机自启")
		autoAct.SetChecked(autostartOn())
		autoAct.Triggered().Attach(func() {
			on := !autoAct.Checked()
			setAutostart(on, pass, port)
			autoAct.SetChecked(on)
		})
		ni.ContextMenu().Actions().Add(autoAct)

		// Service control: lock-screen-capable injection runs in the SYSTEM service.
		svcStatus := walk.NewAction()
		svcStatus.SetText("服务状态：" + serviceState())
		svcStatus.SetEnabled(false)
		ni.ContextMenu().Actions().Add(svcStatus)

		svcInstall := walk.NewAction()
		svcInstall.SetText("安装并启动服务")
		svcInstall.Triggered().Attach(func() {
			elevatedSelf("-install-service", "-pass", pass, "-port", strconv.Itoa(port))
		})
		ni.ContextMenu().Actions().Add(svcInstall)

		svcUninstall := walk.NewAction()
		svcUninstall.SetText("卸载服务")
		svcUninstall.Triggered().Attach(func() { elevatedSelf("-uninstall-service") })
		ni.ContextMenu().Actions().Add(svcUninstall)

		ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())

		quitAct := walk.NewAction()
		quitAct.SetText("退出")
		quitAct.Triggered().Attach(func() { walk.App().Exit(0) })
		ni.ContextMenu().Actions().Add(quitAct)
	} else {
		log.Printf("tray init failed: %v", err)
	}

	// Closing the window hides it to the tray instead of quitting.
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		*canceled = true
		mw.SetVisible(false)
	})

	// Live refresh when devices connect/disconnect (network goroutine → GUI thread).
	refresh := func() {
		model.reload()
		devCount.SetText(fmt.Sprintf("已连接设备 (%d)", len(model.items)))
	}
	reg.SetOnChange(func() { mw.Synchronize(refresh) })
	refresh() // initial population

	// Tick the duration column once a second.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				mw.Synchronize(func() {
					if len(model.items) > 0 {
						model.PublishRowsReset()
					}
				})
			}
		}
	}()

	mw.Run()
}
