//go:build windows

package main

import (
	"io"
	"log"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// statusPipe is the read-only IPC channel the running service exposes so the
// (non-admin) UI can display connected devices while the service owns the port.
// It carries only device metadata (name/addr/since); never the password.
const statusPipe = `\\.\pipe\remotemouse-status`

// serveStatusPipe answers each connection with a JSON snapshot of reg. It runs
// until stop is closed (which closes the listener and unblocks Accept).
func serveStatusPipe(name string, reg *ClientRegistry, stop <-chan struct{}) {
	ln, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: pipeSDDL})
	if err != nil {
		log.Printf("status pipe listen: %v", err)
		return
	}
	go func() {
		<-stop
		ln.Close()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			return // listener closed on stop
		}
		go func(conn net.Conn) {
			defer conn.Close()
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			conn.Write(encodeClients(reg.Snapshot()))
		}(c)
	}
}

// queryStatusPipe dials the service's status pipe and returns its device
// snapshot. Returns an error (handled as "empty") when the service is not
// exposing the pipe yet.
func queryStatusPipe() ([]Client, error) {
	to := 500 * time.Millisecond
	c, err := winio.DialPipe(statusPipe, &to)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	b, err := io.ReadAll(c)
	if err != nil {
		return nil, err
	}
	return decodeClients(b)
}
