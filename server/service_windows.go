//go:build windows

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const svcName = "RemoteMouse"

// setupFileLogger routes the standard logger to
// %ProgramData%\RemoteMouse\<role>.log (writable by SYSTEM, visible from the
// user session) because session-0 and secure-desktop processes have no console.
// Key/text *contents* are never logged.
func setupFileLogger(role string) {
	dir := filepath.Join(os.Getenv("ProgramData"), "RemoteMouse")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, role+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("=== %s log opened (pid %d) ===", role, os.Getpid())
}

func selfPath() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "rmserver.exe"
}

// runService is the -service entry. Under the SCM it runs via svc.Run; if
// launched interactively (dev), it runs the core in the foreground.
func runService(cfg appConfig) {
	setupFileLogger("service")
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("IsWindowsService: %v", err)
	}
	if !isSvc {
		log.Printf("not started by SCM; running service core in foreground (dev)")
		runServiceCore(cfg, make(chan struct{}))
		return
	}
	if err := svc.Run(svcName, &serviceHandler{cfg: cfg}); err != nil {
		log.Fatalf("svc.Run: %v", err)
	}
}

type serviceHandler struct{ cfg appConfig }

func (h *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	stop := make(chan struct{})
	go runServiceCore(h.cfg, stop)
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for cr := range r {
		switch cr.Cmd {
		case svc.Interrogate:
			changes <- cr.CurrentStatus
		case svc.Stop, svc.Shutdown:
			close(stop)
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		}
	}
	return false, 0
}

// runServiceCore loads config, starts the TCP server backed by a pipeInjector,
// stands up the inject pipe, and runs the desktop-follow monitor until stop is
// closed.
func runServiceCore(cfg appConfig, stop <-chan struct{}) {
	enableServicePrivileges()

	if sc, err := readConfig(registry.LOCAL_MACHINE); err == nil {
		cfg.pass, cfg.port = sc.Password, sc.Port
		log.Printf("config: port=%d (password loaded from HKLM)", sc.Port)
	} else {
		log.Printf("readConfig HKLM failed, using flags/defaults: %v", err)
	}

	ln, err := listenInjectPipe(cfg.pipe)
	if err != nil {
		log.Fatalf("listen pipe %s: %v", cfg.pipe, err)
	}
	defer ln.Close()

	pi := newPipeInjector(nil)
	reg := NewClientRegistry()
	srv := &Server{password: cfg.pass, name: cfg.name, inj: pi, reg: reg}
	go func() {
		if err := srv.Listen(cfg.port); err != nil {
			log.Printf("tcp listen: %v", err)
		}
	}()

	mon := &agentMonitor{exe: selfPath(), pipe: cfg.pipe, ln: ln, pi: pi}
	mon.run(stop)
}

// agentMonitor keeps exactly one agent running, bound to the current input
// desktop. On any (session, desktop) change it terminates the old agent and
// spawns a fresh one, then rebinds the pipeInjector's writer to the new pipe
// connection.
type agentMonitor struct {
	exe  string
	pipe string
	ln   net.Listener
	pi   *pipeInjector

	mu      sync.Mutex
	curSess uint32
	curDesk string
	agentH  windows.Handle
	conn    net.Conn
}

func (m *agentMonitor) run(stop <-chan struct{}) {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			m.killAgent()
			return
		case <-tick.C:
			m.reconcile()
		}
	}
}

func (m *agentMonitor) reconcile() {
	sess := activeConsoleSession()
	desk, err := currentInputDesktop()
	if err != nil {
		return // transient during a secure-desktop switch; retry next tick
	}
	m.mu.Lock()
	same := sess == m.curSess && desk == m.curDesk
	m.mu.Unlock()
	if same && m.agentAlive() {
		return
	}
	log.Printf("desktop change -> session=%d desktop=%q (was %d/%q)", sess, desk, m.curSess, m.curDesk)
	m.respawn(sess, desk)
}

func (m *agentMonitor) respawn(sess uint32, desk string) {
	m.killAgent()

	var (
		tok windows.Token
		err error
	)
	switch tokenStrategyFor(desk) {
	case tokenUser:
		tok, err = userTokenForSession(sess)
	default:
		tok, err = winlogonTokenForSession(sess)
	}
	if err != nil {
		log.Printf("token for desktop %q: %v", desk, err)
		return
	}
	defer tok.Close()

	h, err := spawnAgentOnDesktop(tok, m.exe, desk, m.pipe)
	if err != nil {
		log.Printf("spawn agent on %q: %v", desk, err)
		return
	}

	conn, err := m.acceptWithTimeout(5 * time.Second)
	if err != nil {
		log.Printf("agent on %q did not connect: %v", desk, err)
		windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
		return
	}

	m.mu.Lock()
	m.agentH, m.conn = h, conn
	m.curSess, m.curDesk = sess, desk
	m.mu.Unlock()
	m.pi.setWriter(conn)
	log.Printf("agent bound to session=%d desktop=%q", sess, desk)
}

// acceptWithTimeout waits up to d for the freshly spawned agent to connect. On
// timeout the background Accept goroutine completes when the agent eventually
// connects or when the listener closes; acceptable for the POC.
func (m *agentMonitor) acceptWithTimeout(d time.Duration) (net.Conn, error) {
	type result struct {
		c net.Conn
		e error
	}
	ch := make(chan result, 1)
	go func() {
		c, e := m.ln.Accept()
		ch <- result{c, e}
	}()
	select {
	case r := <-ch:
		return r.c, r.e
	case <-time.After(d):
		return nil, fmt.Errorf("accept timeout after %s", d)
	}
}

func (m *agentMonitor) agentAlive() bool {
	m.mu.Lock()
	h := m.agentH
	m.mu.Unlock()
	if h == 0 {
		return false
	}
	ev, err := windows.WaitForSingleObject(h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

func (m *agentMonitor) killAgent() {
	m.mu.Lock()
	h, conn := m.agentH, m.conn
	m.agentH, m.conn = 0, nil
	m.mu.Unlock()

	// Close the conn and terminate the agent BEFORE detaching the writer.
	// pipeInjector.emit holds pi.mu across its pipe Write, so a hung-but-live
	// agent could block that Write while holding pi.mu; calling setWriter(nil)
	// first would then block on the same mutex and wedge the monitor goroutine.
	// Closing conn unblocks the stuck Write, releasing pi.mu so setWriter(nil)
	// can proceed (a concurrent emit to the closed conn just returns an error).
	if conn != nil {
		conn.Close()
	}
	if h != 0 {
		windows.TerminateProcess(h, 0)
		windows.CloseHandle(h)
	}
	m.pi.setWriter(nil)
}

func installService(cfg appConfig) error {
	exepath := selfPath()
	if err := writeConfig(registry.LOCAL_MACHINE, serverConfig{Password: cfg.pass, Port: cfg.port}); err != nil {
		return fmt.Errorf("write HKLM config: %w", err)
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM (run as admin): %w", err)
	}
	defer m.Disconnect()

	if s, err := m.OpenService(svcName); err == nil {
		s.Close()
		return fmt.Errorf("service %s already installed; uninstall first", svcName)
	}
	s, err := m.CreateService(svcName, exepath, mgr.Config{
		DisplayName: "Remote Mouse",
		Description: "Remote Mouse input service (lock-screen capable).",
		StartType:   mgr.StartAutomatic,
	}, "-service")
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()
	fmt.Printf("installed service %q -> %s -service (port %d)\n", svcName, exepath, cfg.port)
	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM (run as admin): %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(svcName)
	if err != nil {
		return fmt.Errorf("service %s not installed: %w", svcName, err)
	}
	defer s.Close()
	s.Control(svc.Stop) // best effort
	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	fmt.Printf("removed service %q\n", svcName)
	return nil
}
