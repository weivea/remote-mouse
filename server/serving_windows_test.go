//go:build windows

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestWantStandalone(t *testing.T) {
	if wantStandalone(true) {
		t.Error("service running -> UI must NOT serve")
	}
	if !wantStandalone(false) {
		t.Error("service not running -> UI must serve")
	}
}

func TestNeedsRebind(t *testing.T) {
	a := serverConfig{Password: "p", Port: 27500, Name: "PC"}
	if needsRebind(a, a) {
		t.Error("identical config must not rebind")
	}
	b := a
	b.Port = 27600
	if !needsRebind(a, b) {
		t.Error("port change must rebind")
	}
	c := a
	c.Password = "x"
	if needsRebind(a, c) {
		t.Error("password-only change must not rebind")
	}
}

func TestNeedsReannounce(t *testing.T) {
	a := serverConfig{Password: "p", Port: 27500, Name: "PC"}
	if needsReannounce(a, a) {
		t.Error("identical config must not re-announce")
	}
	nameChg := a
	nameChg.Name = "TV"
	if !needsReannounce(a, nameChg) {
		t.Error("name change must re-announce")
	}
	portChg := a
	portChg.Port = 1
	if !needsReannounce(a, portChg) {
		t.Error("port change must re-announce")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// dialAuth runs a handshake against 127.0.0.1:port and reports auth_ok.
func dialAuth(t *testing.T, port int, password string) bool {
	t.Helper()
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	enc := json.NewEncoder(c)
	sc := bufio.NewScanner(c)
	enc.Encode(map[string]any{"t": "hello", "name": "P"})
	if !sc.Scan() {
		return false
	}
	var ch struct{ Salt, Nonce string }
	json.Unmarshal(sc.Bytes(), &ch)
	salt, _ := base64.StdEncoding.DecodeString(ch.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(ch.Nonce)
	enc.Encode(map[string]any{"t": "auth", "proof": deriveProof(password, salt, nonce)})
	if !sc.Scan() {
		return false
	}
	var r struct{ T string }
	json.Unmarshal(sc.Bytes(), &r)
	return r.T == "auth_ok"
}

func TestServingControllerLifecycle(t *testing.T) {
	p1 := freePort(t)
	cfg := appConfig{pass: "old", name: "PC", port: p1}
	c := newServingController(cfg, NewClientRegistry())

	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !waitFor(func() bool { return dialAuth(t, p1, "old") }) {
		t.Fatal("service not accepting on p1 after Start")
	}

	// Hot password swap, same port.
	c.Apply(appConfig{pass: "new", name: "PC", port: p1})
	if dialAuth(t, p1, "old") {
		t.Error("old password must fail after Apply")
	}
	if !dialAuth(t, p1, "new") {
		t.Error("new password must work after Apply")
	}

	// Port change -> rebind.
	p2 := freePort(t)
	c.Apply(appConfig{pass: "new", name: "PC", port: p2})
	if !waitFor(func() bool { return dialAuth(t, p2, "new") }) {
		t.Error("must accept on p2 after port change")
	}
	if dialAuth(t, p1, "new") {
		t.Error("old port p1 must be released after successful rebind to p2")
	}

	c.Stop()
	if waitFor(func() bool { return dialAuth(t, p2, "new") }) {
		t.Error("must not accept after Stop")
	}
}

// TestServingControllerRebindFailureRollback forces the new-port bind to fail
// (by pre-occupying the target port) and asserts the controller rolls back to
// the old port, recovers serving there, and keeps cfg (port+name) consistent.
func TestServingControllerRebindFailureRollback(t *testing.T) {
	p1 := freePort(t)
	c := newServingController(appConfig{pass: "pw", name: "PC", port: p1}, NewClientRegistry())
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()
	if !waitFor(func() bool { return dialAuth(t, p1, "pw") }) {
		t.Fatal("not serving on p1 after Start")
	}

	// Occupy p2 (same address form the controller uses) so the rebind fails,
	// forcing rollback to p1.
	p2 := freePort(t)
	blocker, err := net.Listen("tcp", listenAddr(p2))
	if err != nil {
		t.Fatalf("occupy p2: %v", err)
	}
	defer blocker.Close()

	// Change BOTH port (to the occupied p2) and name; password also changes.
	c.Apply(appConfig{pass: "pw2", name: "TV", port: p2})

	// Must recover on the old port with the new password (password swap is
	// independent of the listener).
	if !waitFor(func() bool { return dialAuth(t, p1, "pw2") }) {
		t.Error("must recover serving on old port p1 after rebind failure")
	}
	if !c.isRunning() {
		t.Error("controller must remain running after a successful rollback")
	}
	// cfg must be rolled back so it stays consistent with the still-old mDNS
	// announcement (old port AND old name).
	if c.cfg.Port != p1 {
		t.Errorf("cfg.Port after rollback = %d, want %d", c.cfg.Port, p1)
	}
	if c.cfg.Name != "PC" {
		t.Errorf("cfg.Name after rollback = %q, want \"PC\"", c.cfg.Name)
	}
}
