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
	setupFileLogger("agent")
	log.Printf("agent starting, pipe=%s", cfg.pipe)

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

	inj := newInjector() // winInjector (real SendInput)
	defer inj.Close()
	runAgentLoop(conn, inj)
	log.Printf("agent pipe closed; exiting")
}
