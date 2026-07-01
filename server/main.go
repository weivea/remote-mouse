package main

import (
	"flag"
	"os"
	"runtime"
)

func platform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return runtime.GOOS
	}
}

// appConfig holds parsed CLI configuration and selects the run mode.
type appConfig struct {
	name   string
	pass   string
	port   int
	notray bool

	service          bool
	agent            bool
	absolute         bool
	pipe             string
	installService   bool
	uninstallService bool
	probeDesktop     bool
	standalone       bool
	injectLog        bool
}

// mode resolves the run mode from the flags. install/uninstall win over service
// so an admin can (re)install without the SCM-launched copy interfering.
func (c appConfig) mode() string {
	switch {
	case c.installService:
		return "install-service"
	case c.uninstallService:
		return "uninstall-service"
	case c.service:
		return "service"
	case c.agent:
		return "agent"
	case c.probeDesktop:
		return "probe-desktop"
	case c.standalone:
		return "standalone"
	default:
		return "ui"
	}
}

func main() {
	host, _ := os.Hostname()
	var cfg appConfig
	flag.StringVar(&cfg.name, "name", sanitizeName(host), "device display name")
	flag.StringVar(&cfg.pass, "pass", "1234", "connection password")
	flag.IntVar(&cfg.port, "port", 27500, "TCP control port")
	flag.BoolVar(&cfg.notray, "notray", false, "console mode, no tray icon")
	flag.BoolVar(&cfg.service, "service", false, "run as the Windows service (LocalSystem)")
	flag.BoolVar(&cfg.agent, "agent", false, "run as the injection agent on the input desktop")
	flag.BoolVar(&cfg.absolute, "absolute", false, "agent: use absolute mouse positioning (required on the secure desktop)")
	flag.StringVar(&cfg.pipe, "pipe", `\\.\pipe\remotemouse-inject`, "named pipe for service<->agent IPC")
	flag.BoolVar(&cfg.installService, "install-service", false, "install the Windows service (run as admin)")
	flag.BoolVar(&cfg.uninstallService, "uninstall-service", false, "remove the Windows service (run as admin)")
	flag.BoolVar(&cfg.probeDesktop, "probe-desktop", false, "print the current input desktop in a loop (dev)")
	flag.BoolVar(&cfg.standalone, "standalone", false, "listen + inject in one process, Default desktop only (dev)")
	flag.BoolVar(&cfg.injectLog, "injectlog", false, "agent: verbose per-event injection logging (diagnostic)")
	flag.Parse()

	run(cfg)
}
