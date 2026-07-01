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
	go serveStatusPipe(statusPipe, reg, stop)
	go func() {
		if err := srv.Listen(cfg.port); err != nil {
			log.Printf("tcp listen: %v", err)
		}
	}()

	// The service owns the network, so it must also advertise over mDNS;
	// otherwise the phone's Bonjour discovery finds nothing once installed and
	// the user is forced to type the IP by hand. Non-fatal: manual IP still works.
	if zc := announceMDNS(cfg, newDevID()); zc != nil {
		defer zc.Shutdown()
	}

	mon := &agentMonitor{exe: selfPath(), pipe: cfg.pipe, ln: ln, pi: pi, injectLog: os.Getenv("RM_INJECTLOG") != ""}
	mon.run(stop)
}

// agentMonitor keeps the right set of agents running for the current console
// session's lock state and broadcasts injected events to all of them. Unlocked:
// one agent on the Default desktop. Locked: two agents — Default (user token,
// dismisses the lock curtain) and Winlogon (SYSTEM token, types the PIN on the
// secure desktop). On each tick it reconciles the running set against the
// desired set, spawning/killing agents and rebinding the pipeInjector to a
// fan-out over every live agent connection.
type agentProc struct {
	h    windows.Handle
	conn net.Conn
}

type agentMonitor struct {
	exe  string
	pipe string
	ln   net.Listener
	pi   *pipeInjector

	mu      sync.Mutex
	curSess uint32
	agents  map[string]*agentProc // keyed by desktop name

	injectLog  bool      // pass -injectlog to spawned agents (RM_INJECTLOG at start)
	lastErrLog time.Time // monitor goroutine only; rate-limits detect-error logs
}

func (m *agentMonitor) run(stop <-chan struct{}) {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			m.killAll()
			return
		case <-tick.C:
			m.reconcile()
		}
	}
}

func (m *agentMonitor) reconcile() {
	sess := activeConsoleSession()
	flags, err := sessionLockState(sess)
	if err != nil {
		if time.Since(m.lastErrLog) > 5*time.Second {
			log.Printf("desktop detect (session=%d): %v", sess, err)
			m.lastErrLog = time.Now()
		}
		return // retry next tick
	}
	want := desiredDesktops(flags)

	m.mu.Lock()
	sessChanged := sess != m.curSess
	m.mu.Unlock()
	if sessChanged {
		m.killAll()
		m.mu.Lock()
		m.curSess = sess
		m.mu.Unlock()
	}

	// Build the alive set, reaping any agent whose process has exited so it is
	// treated as missing and respawned below.
	m.mu.Lock()
	alive := make(map[string]bool, len(m.agents))
	var dead []string
	for desk, ap := range m.agents {
		if procAlive(ap.h) {
			alive[desk] = true
		} else {
			dead = append(dead, desk)
		}
	}
	m.mu.Unlock()
	for _, desk := range dead {
		m.stopAgent(desk)
	}

	stop, start := planAgents(alive, want)
	changed := sessChanged || len(dead) > 0 || len(stop) > 0 || len(start) > 0
	for _, desk := range stop {
		log.Printf("agent no longer wanted -> stopping %q (session=%d flags=%d)", desk, sess, flags)
		m.stopAgent(desk)
	}
	for _, desk := range start {
		log.Printf("agent wanted -> starting %q (session=%d flags=%d)", desk, sess, flags)
		m.spawnOne(sess, desk)
	}
	if changed {
		m.rebindWriters()
	}
}

func (m *agentMonitor) spawnOne(sess uint32, desk string) {
	var (
		tok windows.Token
		err error
	)
	strat := tokenStrategyFor(desk)
	switch strat {
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

	h, err := spawnAgentOnDesktop(tok, m.exe, desk, m.pipe, strat != tokenUser, m.injectLog)
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
	if m.agents == nil {
		m.agents = make(map[string]*agentProc)
	}
	m.agents[desk] = &agentProc{h: h, conn: conn}
	m.mu.Unlock()
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

func procAlive(h windows.Handle) bool {
	if h == 0 {
		return false
	}
	ev, err := windows.WaitForSingleObject(h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

// stopAgent terminates the agent on desk and removes it from the set. It closes
// the conn BEFORE the caller rebinds writers: pipeInjector.emit holds pi.mu
// across the fan-out Write, so a hung-but-live agent could block that Write
// while pi.mu is held; closing the conn unblocks the stuck Write so the
// subsequent rebindWriters (which takes pi.mu) cannot wedge the monitor.
func (m *agentMonitor) stopAgent(desk string) {
	m.mu.Lock()
	ap := m.agents[desk]
	delete(m.agents, desk)
	m.mu.Unlock()
	if ap == nil {
		return
	}
	if ap.conn != nil {
		ap.conn.Close()
	}
	if ap.h != 0 {
		windows.TerminateProcess(ap.h, 0)
		windows.CloseHandle(ap.h)
	}
}

// killAll terminates every agent and detaches the injector writer. Conns are
// closed before setWriter(nil) for the same deadlock-avoidance reason as
// stopAgent.
func (m *agentMonitor) killAll() {
	m.mu.Lock()
	agents := m.agents
	m.agents = nil
	m.mu.Unlock()
	for _, ap := range agents {
		if ap.conn != nil {
			ap.conn.Close()
		}
		if ap.h != 0 {
			windows.TerminateProcess(ap.h, 0)
			windows.CloseHandle(ap.h)
		}
	}
	m.pi.setWriter(nil)
}

// rebindWriters points the pipeInjector at a fan-out over every live agent
// connection, so each injected event is broadcast to all agents; only the agent
// on the active input desktop actually injects, the rest no-op with ACCESS_DENIED.
func (m *agentMonitor) rebindWriters() {
	m.mu.Lock()
	ws := make([]deadlineWriter, 0, len(m.agents))
	for _, ap := range m.agents {
		if ap.conn != nil {
			ws = append(ws, ap.conn)
		}
	}
	m.mu.Unlock()
	if len(ws) == 0 {
		m.pi.setWriter(nil)
		return
	}
	m.pi.setWriter(&fanoutWriter{ws: ws, timeout: 2 * time.Second})
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
	if err := s.Start(); err != nil {
		log.Printf("service created but start failed: %v", err)
	}
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

// startService starts the installed service (admin).
func startService() error {
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
	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	fmt.Printf("started service %q\n", svcName)
	return nil
}

// stopService sends a stop control to the service and polls up to 10s for it to
// reach Stopped (admin).
func stopService() error {
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
	st, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for st.State != svc.Stopped && time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return fmt.Errorf("query on stop: %w", err)
		}
	}
	if st.State != svc.Stopped {
		return fmt.Errorf("stop timed out after 10s: service still in state %d", st.State)
	}
	fmt.Printf("stopped service %q\n", svcName)
	return nil
}

// restartService stops then starts the service (admin). A stop error is logged
// but not fatal; startService is always attempted.
func restartService() error {
	if err := stopService(); err != nil {
		log.Printf("restart: stop failed (continuing): %v", err)
	}
	return startService()
}

// setStartType changes the service start type (admin).
func setStartType(t uint32) error {
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
	cfg, err := s.Config()
	if err != nil {
		return fmt.Errorf("read service config: %w", err)
	}
	cfg.StartType = t
	if err := s.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("update start type: %w", err)
	}
	fmt.Printf("set start type of %q to %s\n", svcName, startTypeArg(t))
	return nil
}

// applyConfigElevated persists connection config to HKLM and, if the service is
// running, restarts it so the change takes effect (admin, one UAC).
func applyConfigElevated(cfg appConfig) error {
	if err := writeConfig(registry.LOCAL_MACHINE, serverConfig{Password: cfg.pass, Port: cfg.port, Name: cfg.name}); err != nil {
		return fmt.Errorf("write HKLM config: %w", err)
	}
	if serviceRunning() {
		return restartService()
	}
	fmt.Printf("config saved (service not running; will apply on next start)\n")
	return nil
}
