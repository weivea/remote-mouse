package main

import "testing"

func TestAppConfigMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  appConfig
		want string
	}{
		{"default is ui", appConfig{}, "ui"},
		{"service", appConfig{service: true}, "service"},
		{"agent", appConfig{agent: true}, "agent"},
		{"install", appConfig{installService: true}, "install-service"},
		{"uninstall", appConfig{uninstallService: true}, "uninstall-service"},
		{"probe", appConfig{probeDesktop: true}, "probe-desktop"},
		{"standalone", appConfig{standalone: true}, "standalone"},
		{"install wins over service", appConfig{installService: true, service: true}, "install-service"},
	}
	for _, c := range cases {
		if got := c.cfg.mode(); got != c.want {
			t.Errorf("%s: mode() = %q, want %q", c.name, got, c.want)
		}
	}
}
