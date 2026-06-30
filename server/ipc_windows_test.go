//go:build windows

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInjectPipeRoundTrip(t *testing.T) {
	name := `\\.\pipe\remotemouse-test-` + strconv.Itoa(os.Getpid())
	ln, err := listenInjectPipe(name)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- "accept-error: " + err.Error()
			return
		}
		defer c.Close()
		line, _ := bufio.NewReader(c).ReadString('\n')
		got <- strings.TrimSpace(line)
	}()

	c, err := dialInjectPipe(name)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case s := <-got:
		if s != "ping" {
			t.Fatalf("server read %q, want ping", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server read")
	}
}
