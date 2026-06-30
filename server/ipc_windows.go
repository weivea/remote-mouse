//go:build windows

package main

import (
	"net"

	"github.com/Microsoft/go-winio"
)

// pipeSDDL is the POC DACL for the inject pipe: full access to SYSTEM (SY),
// the Administrators group (BA), and interactive users (IU). This is permissive
// for the POC and is tightened in a follow-up (spec section 15).
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)"

// listenInjectPipe creates the service (server) side of the inject pipe.
func listenInjectPipe(name string) (net.Listener, error) {
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: pipeSDDL})
}

// dialInjectPipe connects the agent (client) side to the inject pipe.
func dialInjectPipe(name string) (net.Conn, error) {
	return winio.DialPipe(name, nil)
}
