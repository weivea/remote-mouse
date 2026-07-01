//go:build windows

package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestListenTCPRetryRecoversWhenPortFreed(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := blocker.Addr().(*net.TCPAddr).Port

	// Free the port shortly, simulating the UI arbiter releasing it.
	go func() {
		time.Sleep(400 * time.Millisecond)
		blocker.Close()
	}()

	ln, err := listenTCPRetry(port, 3*time.Second)
	if err != nil {
		t.Fatalf("expected retry to bind after port freed, got %v", err)
	}
	ln.Close()
}

func TestListenTCPRetryTimesOut(t *testing.T) {
	blocker, err := net.Listen("tcp", fmt.Sprintf(":%d", 0))
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port

	if _, err := listenTCPRetry(port, 500*time.Millisecond); err == nil {
		t.Fatal("expected timeout error when the port stays occupied")
	}
}
