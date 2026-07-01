//go:build windows

package main

import (
	"log"
	"net"
	"time"
)

// runAgent connects to the service's inject pipe and applies forwarded events
// via SendInput on whatever desktop this process was spawned onto. The service
// terminates this process when the input desktop changes, so on pipe close the
// agent simply exits.
func runAgent(cfg appConfig) {
	// Split logs per desktop so the two lock-screen agents don't interleave:
	// the Default/user agent (curtain) -> agent-user.log, the Winlogon/secure
	// agent (PIN) -> agent-secure.log.
	role := "agent-user"
	if cfg.absolute {
		role = "agent-secure"
	}
	setupFileLogger(role)
	log.Printf("agent starting, pipe=%s absolute=%v", cfg.pipe, cfg.absolute)

	var conn net.Conn
	for i := 0; i < 20; i++ { // ~5s of startup retries while the listener comes up
		c, err := dialInjectPipe(cfg.pipe)
		if err == nil {
			conn = c
			break
		}
		log.Printf("agent dial failed (attempt %d): %v", i+1, err)
		time.Sleep(250 * time.Millisecond)
	}
	if conn == nil {
		log.Printf("agent giving up: could not connect to %s", cfg.pipe)
		return
	}
	defer conn.Close()
	log.Printf("agent connected; injecting on this desktop")

	inj := newInjector() // winInjector (real SendInput), relative mouse
	if cfg.absolute {
		inj = newSecureInjector() // absolute mouse for the secure desktop
	}
	// Per-event injection logging is opt-in: the service passes -injectlog when
	// it was started with RM_INJECTLOG set, so production stays quiet while the
	// two lock-screen desktops (curtain click on Default, PIN text on Winlogon)
	// can still be traced on demand.
	if cfg.injectLog {
		setVerboseInject(true)
	}
	defer inj.Close()
	runAgentLoop(conn, inj)
	log.Printf("agent pipe closed; exiting")
}
