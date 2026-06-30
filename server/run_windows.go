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
	case "service":
		runService(cfg)
	case "agent":
		runAgent(cfg)
	default: // "ui"
		serveUI(cfg)
	}
}
