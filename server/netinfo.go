package main

import (
	"net"
	"strings"
)

// HostIPs holds the recommended LAN IP and other usable IPv4 addresses.
type HostIPs struct {
	Primary   string
	Secondary []string
}

// virtualKeywords match interface names of non-physical adapters.
var virtualKeywords = []string{
	"vethernet", "hyper-v", "vmware", "virtualbox", "vbox",
	"loopback", "bluetooth", "wsl", "tailscale", "docker", "tun", "tap",
}

func isVirtualName(name string) bool {
	n := strings.ToLower(name)
	for _, k := range virtualKeywords {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

// ifaceAddrs pairs an interface with its IPs, decoupled from net for testability.
type ifaceAddrs struct {
	Name  string
	Up    bool
	Loop  bool
	Addrs []net.IP
}

// pickIPs chooses the recommended Primary and the Secondary list.
func pickIPs(ifaces []ifaceAddrs) HostIPs {
	var physPrivate, others []string
	for _, f := range ifaces {
		if !f.Up || f.Loop {
			continue
		}
		virtual := isVirtualName(f.Name)
		for _, ip := range f.Addrs {
			v4 := ip.To4()
			if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() {
				continue // skip non-IPv4, loopback, APIPA 169.254.0.0/16
			}
			s := v4.String()
			if !virtual && v4.IsPrivate() {
				physPrivate = append(physPrivate, s)
			} else {
				others = append(others, s)
			}
		}
	}
	res := HostIPs{}
	switch {
	case len(physPrivate) > 0:
		res.Primary = physPrivate[0]
		res.Secondary = append(append([]string{}, physPrivate[1:]...), others...)
	case len(others) > 0:
		res.Primary = others[0]
		res.Secondary = others[1:]
	}
	return res
}

// DetectIPs reads real interfaces and returns recommended IPs.
func DetectIPs() HostIPs {
	list, err := net.Interfaces()
	if err != nil {
		return HostIPs{}
	}
	var ifaces []ifaceAddrs
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
		ifaces = append(ifaces, fa)
	}
	return pickIPs(ifaces)
}
