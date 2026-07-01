package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// drive a full hello→challenge→auth handshake over net.Pipe and assert the
// registry reflects connect then disconnect.
func TestHandleRegistersAndDeregisters(t *testing.T) {
	reg := NewClientRegistry()
	s := &Server{password: "pw", name: "srv", inj: newInjector(), reg: reg}

	cli, srvConn := net.Pipe()
	go s.handle(srvConn)
	defer cli.Close()
	cli.SetDeadline(time.Now().Add(3 * time.Second))

	enc := json.NewEncoder(cli)
	sc := bufio.NewScanner(cli)

	// hello
	if err := enc.Encode(map[string]any{"t": "hello", "name": "TestPhone"}); err != nil {
		t.Fatal(err)
	}
	// challenge
	if !sc.Scan() {
		t.Fatal("no challenge")
	}
	var ch struct {
		Salt  string `json:"salt"`
		Nonce string `json:"nonce"`
	}
	json.Unmarshal(sc.Bytes(), &ch)
	salt, _ := base64.StdEncoding.DecodeString(ch.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(ch.Nonce)
	// auth
	if err := enc.Encode(map[string]any{"t": "auth", "proof": deriveProof("pw", salt, nonce)}); err != nil {
		t.Fatal(err)
	}
	// auth_ok
	if !sc.Scan() {
		t.Fatal("no auth_ok")
	}

	if !waitFor(func() bool { return len(reg.Snapshot()) == 1 }) {
		t.Fatalf("registry not populated, got %d", len(reg.Snapshot()))
	}
	if snap := reg.Snapshot(); snap[0].Name != "TestPhone" {
		t.Errorf("name = %q, want TestPhone", snap[0].Name)
	}

	cli.Close()
	if !waitFor(func() bool { return len(reg.Snapshot()) == 0 }) {
		t.Errorf("registry not cleared after disconnect, got %d", len(reg.Snapshot()))
	}
}

func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// runHandshake performs hello→challenge→auth against s over an in-memory pipe
// and reports whether the server accepted (auth_ok). It closes the client when
// done.
func runHandshake(t *testing.T, s *Server, password string) bool {
	t.Helper()
	cli, srvConn := net.Pipe()
	go s.handle(srvConn)
	defer cli.Close()
	cli.SetDeadline(time.Now().Add(3 * time.Second))

	enc := json.NewEncoder(cli)
	sc := bufio.NewScanner(cli)

	if err := enc.Encode(map[string]any{"t": "hello", "name": "P"}); err != nil {
		t.Fatal(err)
	}
	if !sc.Scan() {
		t.Fatal("no challenge")
	}
	var ch struct {
		Salt  string `json:"salt"`
		Nonce string `json:"nonce"`
	}
	json.Unmarshal(sc.Bytes(), &ch)
	salt, _ := base64.StdEncoding.DecodeString(ch.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(ch.Nonce)
	if err := enc.Encode(map[string]any{"t": "auth", "proof": deriveProof(password, salt, nonce)}); err != nil {
		t.Fatal(err)
	}
	if !sc.Scan() {
		t.Fatal("no reply")
	}
	var reply struct {
		T string `json:"t"`
	}
	json.Unmarshal(sc.Bytes(), &reply)
	return reply.T == "auth_ok"
}

func TestSetPasswordHotSwap(t *testing.T) {
	s := &Server{password: "old", name: "srv", inj: newInjector(), reg: NewClientRegistry()}
	if !runHandshake(t, s, "old") {
		t.Fatal("old password should authenticate before swap")
	}
	s.SetPassword("new")
	if runHandshake(t, s, "old") {
		t.Error("old password must fail after swap")
	}
	if !runHandshake(t, s, "new") {
		t.Error("new password must authenticate after swap")
	}
}
