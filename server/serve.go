package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"

	"github.com/grandcat/zeroconf"
)

// serveStandalone runs the original single-process behavior: announce over
// mDNS, listen for clients, inject locally via the platform injector, and show
// the platform UI. It is the only mode on non-Windows platforms and the
// "-standalone" dev mode on Windows.
func serveStandalone(cfg appConfig) {
	reg := NewClientRegistry()
	srv := &Server{password: cfg.pass, name: cfg.name, inj: newInjector(), reg: reg}
	defer srv.inj.Close()

	id := newDevID()
	if zc := announceMDNS(cfg, id); zc != nil {
		defer zc.Shutdown()
	}

	log.Printf("password=%q  platform=%s  devid=%s", cfg.pass, platform(), id)
	go func() {
		if err := srv.Listen(cfg.port); err != nil {
			log.Fatal(err)
		}
	}()
	ips := DetectIPs()
	runUI(cfg.notray, cfg.pass, cfg.port, ips, reg)
}

func newDevID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// announceMDNS registers _remotemouse._tcp; returns nil on failure (manual IP
// entry still works).
func announceMDNS(cfg appConfig, id string) *zeroconf.Server {
	zc, err := zeroconf.Register(cfg.name, "_remotemouse._tcp", "local.", cfg.port, []string{
		"name=" + cfg.name, "platform=" + platform(), "ver=0.1", "devid=" + id,
	}, nil)
	if err != nil {
		log.Printf("mDNS announce failed (manual IP still works): %v", err)
		return nil
	}
	log.Printf("mDNS announce: %q _remotemouse._tcp on %d", cfg.name, cfg.port)
	return zc
}
