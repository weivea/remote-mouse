//go:build windows

package main

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/libp2p/zeroconf/v2"
)

// wantStandalone reports whether the UI process should own the TCP port and
// serve directly. When the RemoteMouse service is running it owns the port, so
// the UI must stay a controller and NOT serve.
func wantStandalone(serviceRunning bool) bool { return !serviceRunning }

// needsRebind reports whether applying newCfg over old requires re-binding the
// TCP listener (only a port change does).
func needsRebind(old, new serverConfig) bool { return old.Port != new.Port }

// needsReannounce reports whether the mDNS advertisement must be refreshed
// (port or display name changed).
func needsReannounce(old, new serverConfig) bool {
	return old.Port != new.Port || old.Name != new.Name
}

// servingController owns the in-process standalone serving stack (Server +
// TCP listener + mDNS). It can be Start()ed / Stop()ped to acquire/release the
// port as the service comes and goes, and Apply()'d to hot-change config
// without dropping already-connected phones. Not safe for concurrent Start/Stop
// from multiple goroutines; drive it from the single StatusTicker goroutine.
type servingController struct {
	mu      sync.Mutex
	cfg     serverConfig // current effective config (Password/Port/Name)
	reg     *ClientRegistry
	srv     *Server
	inj     Injector
	devID   string
	ln      net.Listener
	zc      *zeroconf.Server
	running bool
}

func newServingController(cfg appConfig, reg *ClientRegistry) *servingController {
	return &servingController{
		cfg:   serverConfig{Password: cfg.pass, Port: cfg.port, Name: cfg.name},
		reg:   reg,
		devID: newDevID(),
	}
}

// Start binds the listener, announces mDNS, and serves. No-op if running.
func (c *servingController) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	if c.inj == nil {
		c.inj = newInjector()
	}
	if c.srv == nil {
		c.srv = &Server{password: c.cfg.Password, name: c.cfg.Name, inj: c.inj, reg: c.reg}
	} else {
		c.srv.SetPassword(c.cfg.Password)
		c.srv.SetName(c.cfg.Name)
	}
	ln, err := net.Listen("tcp", listenAddr(c.cfg.Port))
	if err != nil {
		return err
	}
	c.ln = ln
	c.zc = announceMDNS(c.announceCfg(), c.devID)
	go c.srv.Serve(ln)
	c.running = true
	log.Printf("serving controller: started on tcp/%d", c.cfg.Port)
	return nil
}

// Stop releases the port and mDNS. Already-connected phones may linger until
// they disconnect; new connections stop immediately. No-op if not running.
func (c *servingController) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return
	}
	if c.zc != nil {
		c.zc.Shutdown()
		c.zc = nil
	}
	if c.ln != nil {
		c.ln.Close()
		c.ln = nil
	}
	c.running = false
	log.Printf("serving controller: stopped (released tcp/%d)", c.cfg.Port)
}

// Apply hot-applies a new config. Password swaps in place; a port change
// rebinds a fresh listener (existing connections survive); a port/name change
// re-announces mDNS. When not running it just records the new config.
func (c *servingController) Apply(next appConfig) {
	nc := serverConfig{Password: next.pass, Port: next.port, Name: next.name}
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.cfg
	c.cfg = nc
	if !c.running {
		return
	}
	c.srv.SetPassword(nc.Password)
	c.srv.SetName(nc.Name)
	if needsRebind(old, nc) {
		if c.ln != nil {
			c.ln.Close()
		}
		ln, err := net.Listen("tcp", listenAddr(nc.Port))
		if err != nil {
			log.Printf("serving controller: rebind tcp/%d failed: %v", nc.Port, err)
			// Keep old listener closed; re-open old port as best effort.
			if ln2, e2 := net.Listen("tcp", listenAddr(old.Port)); e2 == nil {
				c.ln = ln2
				c.cfg.Port = old.Port
				go c.srv.Serve(ln2)
			}
			return
		}
		c.ln = ln
		go c.srv.Serve(ln)
	}
	if needsReannounce(old, nc) {
		if c.zc != nil {
			c.zc.Shutdown()
		}
		c.zc = announceMDNS(c.announceCfg(), c.devID)
	}
}

// isRunning reports whether the controller currently owns the port.
func (c *servingController) isRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *servingController) announceCfg() appConfig {
	return appConfig{name: c.cfg.Name, port: c.cfg.Port}
}

func listenAddr(port int) string { return fmt.Sprintf(":%d", port) }
