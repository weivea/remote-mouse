package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log"
	"os"
	"runtime"

	"github.com/grandcat/zeroconf"
)

func platform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return runtime.GOOS
	}
}

func main() {
	host, _ := os.Hostname()
	name := flag.String("name", sanitizeName(host), "device display name")
	pass := flag.String("pass", "1234", "connection password")
	port := flag.Int("port", 27500, "TCP control port")
	flag.Parse()

	devid := make([]byte, 8)
	rand.Read(devid)
	id := hex.EncodeToString(devid)

	srv := &Server{password: *pass, name: *name, inj: newInjector()}
	defer srv.inj.Close()

	zc, err := zeroconf.Register(*name, "_remotemouse._tcp", "local.", *port, []string{
		"name=" + *name, "platform=" + platform(), "ver=0.1", "devid=" + id,
	}, nil)
	if err != nil {
		log.Printf("mDNS announce failed (manual IP still works): %v", err)
	} else {
		defer zc.Shutdown()
		log.Printf("mDNS announce: %q _remotemouse._tcp on %d", *name, *port)
	}

	log.Printf("password=%q  platform=%s  devid=%s", *pass, platform(), id)
	if err := srv.Listen(*port); err != nil {
		log.Fatal(err)
	}
}
