package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net"

	"github.com/libp2p/zeroconf/v2"
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
// entry still works). It advertises only on physical, up interfaces so the A
// records don't include virtual-adapter IPs (Hyper-V/WSL/Docker/VPN) the phone
// can't route to.
func announceMDNS(cfg appConfig, id string) *zeroconf.Server {
	ifaces, names := announceIfaces()
	zc, err := zeroconf.Register(cfg.name, "_remotemouse._tcp", "local.", cfg.port, []string{
		"name=" + cfg.name, "platform=" + platform(), "ver=0.1", "devid=" + id,
	}, ifaces)
	if err != nil {
		log.Printf("mDNS announce failed (manual IP still works): %v", err)
		return nil
	}
	if names == nil {
		names = []string{"all"} // zeroconf falls back to every interface
	}
	log.Printf("mDNS announce: %q _remotemouse._tcp on %d (ifaces=%v)", cfg.name, cfg.port, names)
	return zc
}

// keepAnnounceIface reports whether an interface should carry mDNS
// announcements: it must be up, non-loopback, physical (not a Hyper-V/WSL/
// Docker/VPN virtual adapter), and hold a routable IPv4 the phone can reach.
func keepAnnounceIface(f ifaceAddrs) bool {
	return f.Up && !f.Loop && !isVirtualName(f.Name) && hasRoutableV4(f.Addrs)
}

// hasRoutableV4 reports whether addrs contains a non-loopback, non-APIPA IPv4.
func hasRoutableV4(addrs []net.IP) bool {
	for _, ip := range addrs {
		v4 := ip.To4()
		if v4 != nil && !v4.IsLoopback() && !v4.IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

// announceIfaces returns the interfaces to advertise on and their names for
// logging. Returns (nil, nil) when none qualify, so zeroconf falls back to all
// interfaces and discovery still works on unusual setups.
func announceIfaces() ([]net.Interface, []string) {
	list, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}
	var ifs []net.Interface
	var names []string
	for _, in := range list {
		fa := ifaceAddrs{
			Name: in.Name,
			Up:   in.Flags&net.FlagUp != 0,
			Loop: in.Flags&net.FlagLoopback != 0,
		}
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				fa.Addrs = append(fa.Addrs, ipn.IP)
			}
		}
		if !keepAnnounceIface(fa) {
			continue
		}
		ifs = append(ifs, in)
		names = append(names, in.Name)
	}
	if len(ifs) == 0 {
		return nil, nil
	}
	return ifs, names
}
