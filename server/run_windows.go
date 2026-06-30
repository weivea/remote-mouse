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
		log.Fatalf("install-service not yet implemented") // Task 8
	case "uninstall-service":
		log.Fatalf("uninstall-service not yet implemented") // Task 8
	case "service":
		log.Fatalf("service not yet implemented") // Task 8
	case "agent":
		log.Fatalf("agent not yet implemented") // Task 8
	default: // "ui": today's listen+inject+tray; becomes UI-only in Task 9
		serveStandalone(cfg)
	}
}
