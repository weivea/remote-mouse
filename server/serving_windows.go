//go:build windows

package main

// wantStandalone reports whether the UI process should own the TCP port and
// serve directly. When the RemoteMouse service is running it owns the port, so
// the UI must stay a controller and NOT serve.
func wantStandalone(serviceRunning bool) bool { return !serviceRunning }

// needsRebind reports whether applying newCfg over old requires re-binding the
// TCP listener (only a port change does).
func needsRebind(old, new serverConfig) bool { return old.Port != new.Port }

// needsReannounce reports whether the mDNS advertisement must be refreshed
// (port or display name changed).
func needsReannounce(old, new serverConfig) bool {
	return old.Port != new.Port || old.Name != new.Name
}
