//go:build windows

package main

import "golang.org/x/sys/windows/registry"

const configPath = `SOFTWARE\RemoteMouse`

// serverConfig is the connection config shared by the service (reads HKLM) and
// the tray UI (reads/writes HKLM, elevated). It never stores the user's PIN.
type serverConfig struct {
	Password string
	Port     int
}

// readConfig reads connection config from root\SOFTWARE\RemoteMouse. On any
// error the returned config still carries the built-in defaults.
func readConfig(root registry.Key) (serverConfig, error) {
	cfg := serverConfig{Password: "1234", Port: 27500}
	k, err := registry.OpenKey(root, configPath, registry.QUERY_VALUE)
	if err != nil {
		return cfg, err
	}
	defer k.Close()
	if p, _, err := k.GetStringValue("Password"); err == nil {
		cfg.Password = p
	}
	if n, _, err := k.GetIntegerValue("Port"); err == nil {
		cfg.Port = int(n)
	}
	return cfg, nil
}

// writeConfig persists config under root\SOFTWARE\RemoteMouse, creating the key.
func writeConfig(root registry.Key, cfg serverConfig) error {
	k, _, err := registry.CreateKey(root, configPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("Password", cfg.Password); err != nil {
		return err
	}
	return k.SetDWordValue("Port", uint32(cfg.Port))
}
