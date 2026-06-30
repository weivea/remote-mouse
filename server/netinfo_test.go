package main

import (
	"net"
	"testing"
)

func ips(ss ...string) []net.IP {
	var out []net.IP
	for _, s := range ss {
		out = append(out, net.ParseIP(s))
	}
	return out
}

func TestPickIPsPrefersPhysicalPrivate(t *testing.T) {
	got := pickIPs([]ifaceAddrs{
		{Name: "vEthernet (Default Switch)", Up: true, Addrs: ips("172.17.48.1")},
		{Name: "Ethernet", Up: true, Addrs: ips("10.32.86.111")},
		{Name: "Wi-Fi", Up: true, Addrs: ips("10.95.140.97")},
	})
	if got.Primary != "10.32.86.111" {
		t.Errorf("Primary = %q, want 10.32.86.111", got.Primary)
	}
	// vEthernet address must be classified as secondary, not dropped
	found := false
	for _, s := range got.Secondary {
		if s == "172.17.48.1" {
			found = true
		}
	}
	if !found {
		t.Errorf("Secondary = %v, want it to contain 172.17.48.1", got.Secondary)
	}
}

func TestPickIPsSkipsLoopbackDownAPIPA(t *testing.T) {
	got := pickIPs([]ifaceAddrs{
		{Name: "lo", Up: true, Loop: true, Addrs: ips("127.0.0.1")},
		{Name: "Ethernet", Up: false, Addrs: ips("10.0.0.5")},   // down
		{Name: "Wi-Fi", Up: true, Addrs: ips("169.254.1.2")},    // APIPA only
		{Name: "Wi-Fi 2", Up: true, Addrs: ips("192.168.1.50")}, // valid
	})
	if got.Primary != "192.168.1.50" {
		t.Errorf("Primary = %q, want 192.168.1.50", got.Primary)
	}
}

func TestPickIPsFallbackToNonPrivate(t *testing.T) {
	got := pickIPs([]ifaceAddrs{
		{Name: "Ethernet", Up: true, Addrs: ips("100.64.0.2")}, // CGNAT, not RFC1918
	})
	if got.Primary != "100.64.0.2" {
		t.Errorf("Primary = %q, want fallback 100.64.0.2", got.Primary)
	}
}

func TestPickIPsEmpty(t *testing.T) {
	got := pickIPs(nil)
	if got.Primary != "" || len(got.Secondary) != 0 {
		t.Errorf("want empty HostIPs, got %+v", got)
	}
}
