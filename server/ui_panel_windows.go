//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// runControlPanel shows the integrated control panel: editable connection
// settings, live service management, and the connected-devices table. It owns
// a StatusTicker goroutine that (1) reads service state + start type, (2) drives
// the ServingArbiter to acquire/release the port, and (3) refreshes the device
// table from the local registry (standalone) or the status pipe (service mode).
func runControlPanel(cfg appConfig, ctrl *servingController, reg *ClientRegistry, ips HostIPs) {
	if cfg.notray {
		// Headless: still run the arbiter loop so serving follows the service.
		runArbiterHeadless(ctrl)
		return
	}
	runtime.LockOSThread()

	primary := ips.Primary
	if primary == "" {
		primary = "未检测到局域网 IP"
	}
	secText := strings.Join(ips.Secondary, "   ")

	var mw *walk.MainWindow
	var portEdit, passEdit, nameEdit *walk.LineEdit
	var secLabel, devCount, statusLabel *walk.Label
	var startCombo *walk.ComboBox
	var btnInstall, btnStart, btnStop, btnUninstall, btnApply, btnShowPass *walk.PushButton
	model := &deviceModel{reg: reg}

	// applyGuard suppresses the combo's OnCurrentIndexChanged while the ticker
	// programmatically sets its index.
	applyGuard := false

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "RemoteMouse 控制面板",
		MinSize:  Size{Width: 460, Height: 520},
		Size:     Size{Width: 480, Height: 560},
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
					LineEdit{AssignTo: &portEdit, Text: strconv.Itoa(cfg.port)},
					PushButton{Text: "复制", OnClicked: func() { copyClip(portEdit.Text()) }},

					Label{Text: "密码"},
					LineEdit{AssignTo: &passEdit, Text: cfg.pass, PasswordMode: true},
					PushButton{AssignTo: &btnShowPass, Text: "显示", OnClicked: func() {
						show := passEdit.PasswordMode()
						passEdit.SetPasswordMode(!show)
						if show {
							btnShowPass.SetText("隐藏")
						} else {
							btnShowPass.SetText("显示")
						}
					}},

					Label{Text: "设备名"},
					LineEdit{AssignTo: &nameEdit, Text: cfg.name},
					PushButton{Text: "复制", OnClicked: func() { copyClip(nameEdit.Text()) }},

					Label{Text: "其他地址"},
					Label{AssignTo: &secLabel, Text: secText, Visible: false},
					PushButton{Text: "展开", Enabled: secText != "", OnClicked: func() {
						secLabel.SetVisible(!secLabel.Visible())
					}},

					Label{Text: ""},
					PushButton{AssignTo: &btnApply, Text: "保存并应用"},
					Label{Text: "改后点这里生效"},
				},
			},
			GroupBox{
				Title:  "服务管理",
				Layout: Grid{Columns: 4},
				Children: []Widget{
					Label{Text: "状态"},
					Label{AssignTo: &statusLabel, Text: "查询中…", Font: Font{Bold: true}, ColumnSpan: 3},

					Label{Text: "启动类型"},
					ComboBox{AssignTo: &startCombo, Model: startTypeLabels(), ColumnSpan: 3},

					PushButton{AssignTo: &btnInstall, Text: "安装并启动"},
					PushButton{AssignTo: &btnStart, Text: "启动"},
					PushButton{AssignTo: &btnStop, Text: "停止"},
					PushButton{AssignTo: &btnUninstall, Text: "卸载"},

					Label{Text: "管理操作需要管理员权限（会弹 UAC）", ColumnSpan: 4},
				},
			},
			Label{AssignTo: &devCount, Text: "已连接设备 (0)"},
			TableView{
				MinSize: Size{Height: 150},
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
		log.Printf("control panel init failed, running headless: %v", err)
		runArbiterHeadless(ctrl)
		return
	}

	// readConn validates the editable port/password fields, showing a warning
	// and returning ok=false when either is invalid. Shared by Save & Apply,
	// Install, and login-autostart so none hands bad input to the elevated child
	// (which would exit silently on a flag parse error).
	readConn := func() (port int, pass string, ok bool) {
		port, err := strconv.Atoi(strings.TrimSpace(portEdit.Text()))
		if err != nil || port < 1 || port > 65535 {
			walk.MsgBox(mw, "端口无效", "端口需为 1–65535 的整数。", walk.MsgBoxIconWarning)
			return 0, "", false
		}
		pass = passEdit.Text()
		if pass == "" {
			walk.MsgBox(mw, "密码为空", "密码不能为空。", walk.MsgBoxIconWarning)
			return 0, "", false
		}
		return port, pass, true
	}

	// --- Save & Apply ---
	btnApply.Clicked().Attach(func() {
		port, pass, ok := readConn()
		if !ok {
			return
		}
		next := appConfig{pass: pass, port: port, name: strings.TrimSpace(nameEdit.Text())}
		if serviceInstalled() {
			// Persist to HKLM (+ restart if running) via one elevated call.
			elevatedSelf("-apply-config", "-pass", pass, "-port", strconv.Itoa(port), "-name", next.name)
			// If we are currently the one serving (installed but stopped), also
			// hot-apply so the live standalone server reflects the change.
			ctrl.Apply(next)
		} else {
			ctrl.Apply(next)
			if autostartOn() {
				setAutostartNamed(true, pass, port, next.name) // rewrite Run command with new values
			}
		}
	})

	// --- Service buttons (elevated) ---
	btnInstall.Clicked().Attach(func() {
		port, pass, ok := readConn()
		if !ok {
			return
		}
		elevatedSelf("-install-service", "-pass", pass, "-port", strconv.Itoa(port), "-name", strings.TrimSpace(nameEdit.Text()))
	})
	btnStart.Clicked().Attach(func() { elevatedSelf("-start-service") })
	btnStop.Clicked().Attach(func() { elevatedSelf("-stop-service") })
	btnUninstall.Clicked().Attach(func() { elevatedSelf("-uninstall-service") })

	// --- Start-type combo (elevated) ---
	startCombo.CurrentIndexChanged().Attach(func() {
		if applyGuard {
			return
		}
		t := startTypeByIndex(startCombo.CurrentIndex())
		elevatedSelf("-set-start-type", startTypeArg(t))
	})

	// Tray icon + minimal menu.
	if ni, err := walk.NewNotifyIcon(mw); err == nil {
		defer ni.Dispose()
		if ic := loadIcon(); ic != nil {
			ni.SetIcon(ic)
			mw.SetIcon(ic)
		}
		ni.SetToolTip("RemoteMouse  " + primary)
		ni.SetVisible(true)
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

		loginAct := walk.NewAction()
		loginAct.SetText("登录时打开界面")
		loginAct.SetChecked(autostartOn())
		loginAct.Triggered().Attach(func() {
			on := !loginAct.Checked()
			if on {
				port, pass, ok := readConn()
				if !ok {
					return
				}
				setAutostartNamed(true, pass, port, strings.TrimSpace(nameEdit.Text()))
			} else {
				setAutostart(false, "", 0)
			}
			loginAct.SetChecked(on)
		})
		ni.ContextMenu().Actions().Add(loginAct)

		ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())
		quitAct := walk.NewAction()
		quitAct.SetText("退出")
		quitAct.Triggered().Attach(func() { walk.App().Exit(0) })
		ni.ContextMenu().Actions().Add(quitAct)
	} else {
		log.Printf("tray init failed: %v", err)
	}

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		*canceled = true
		mw.SetVisible(false)
	})

	// Live device refresh for standalone mode (push). Service mode is polled by
	// the ticker below.
	reg.SetOnChange(func() {
		mw.Synchronize(func() {
			if ctrl.isRunning() {
				model.items = reg.Snapshot()
				model.PublishRowsReset()
				devCount.SetText(fmt.Sprintf("已连接设备 (%d)", len(model.items)))
			}
		})
	})

	// StatusTicker: arbitrate serving + refresh status/devices every second.
	// All blocking I/O (SCM, pipe) happens here on a background goroutine; UI
	// mutations go through mw.Synchronize.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		tick := func() {
			running := serviceRunning()
			// Arbitrate: own the port iff the service is not running.
			if wantStandalone(running) {
				ctrl.Start()
			} else {
				ctrl.Stop()
			}
			state := serviceState()
			stype, installed := serviceStartType()
			var devices []Client
			if ctrl.isRunning() {
				devices = reg.Snapshot()
			} else if list, err := queryStatusPipe(); err == nil {
				devices = list
			}
			mw.Synchronize(func() {
				applyPanelState(panelWidgets{
					statusLabel: statusLabel, startCombo: startCombo,
					btnInstall: btnInstall, btnStart: btnStart, btnStop: btnStop,
					btnUninstall: btnUninstall, devCount: devCount, model: model,
				}, state, stype, installed, devices, &applyGuard)
			})
		}
		tick() // immediate first paint
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				tick()
			}
		}
	}()

	mw.Run()
}

// panelWidgets bundles the widgets the ticker updates.
type panelWidgets struct {
	statusLabel  *walk.Label
	startCombo   *walk.ComboBox
	btnInstall   *walk.PushButton
	btnStart     *walk.PushButton
	btnStop      *walk.PushButton
	btnUninstall *walk.PushButton
	devCount     *walk.Label
	model        *deviceModel
}

// applyPanelState updates all live widgets from a status snapshot. Runs on the
// GUI thread. guard is toggled around the combo write so the change handler
// does not treat a programmatic set as a user action.
func applyPanelState(w panelWidgets, state string, stype uint32, installed bool, devices []Client, guard *bool) {
	if installed {
		w.statusLabel.SetText(fmt.Sprintf("● %s（%s）", state, startTypeLabel(stype)))
	} else {
		w.statusLabel.SetText("● 未安装")
	}

	*guard = true
	if idx := startTypeIndex(stype); installed && idx >= 0 {
		w.startCombo.SetCurrentIndex(idx)
		w.startCombo.SetEnabled(true)
	} else {
		w.startCombo.SetEnabled(false)
	}
	*guard = false

	// Button matrix.
	w.btnInstall.SetEnabled(!installed)
	w.btnStart.SetEnabled(installed && state == "已停止")
	w.btnStop.SetEnabled(installed && state == "运行中")
	w.btnUninstall.SetEnabled(installed)

	w.model.items = devices
	w.model.PublishRowsReset()
	w.devCount.SetText(fmt.Sprintf("已连接设备 (%d)", len(devices)))
}

// runArbiterHeadless keeps serving following the service state without a window
// (notray). Blocks forever.
func runArbiterHeadless(ctrl *servingController) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		if wantStandalone(serviceRunning()) {
			ctrl.Start()
		} else {
			ctrl.Stop()
		}
	}
}
