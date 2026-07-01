//go:build windows

package main

import "testing"

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
