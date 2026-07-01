//go:build windows

package main

import "log"

// run dispatches by mode. The log.Fatalf branches are temporary runtime guards,
// replaced as each Windows mode lands in later tasks.
func run(cfg appConfig) {
	switch cfg.mode() {
	case "standalone":
		serveStandalone(cfg)
	case "probe-desktop":
		probeDesktop()
	case "install-service":
		if err := installService(cfg); err != nil {
			log.Fatalf("install-service: %v", err)
		}
	case "uninstall-service":
		if err := uninstallService(); err != nil {
			log.Fatalf("uninstall-service: %v", err)
		}
	case "start-service":
		if err := startService(); err != nil {
			log.Fatalf("start-service: %v", err)
		}
	case "stop-service":
		if err := stopService(); err != nil {
			log.Fatalf("stop-service: %v", err)
		}
	case "set-start-type":
		t, ok := startTypeFromArg(cfg.setStartType)
		if !ok {
			log.Fatalf("set-start-type: invalid value %q (want auto|manual|disabled)", cfg.setStartType)
		}
		if err := setStartType(t); err != nil {
			log.Fatalf("set-start-type: %v", err)
		}
	case "apply-config":
		if err := applyConfigElevated(cfg); err != nil {
			log.Fatalf("apply-config: %v", err)
		}
	case "service":
		runService(cfg)
	case "agent":
		runAgent(cfg)
	default: // "ui"
		serveUI(cfg)
	}
}
