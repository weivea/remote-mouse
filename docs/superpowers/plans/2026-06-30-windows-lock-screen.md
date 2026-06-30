# Windows Lock-Screen Injection (M5 POC) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the iPhone client type the Windows Hello PIN on the lock/login secure desktop by introducing a LocalSystem service that owns the network and spawns a per-desktop injection agent.

**Architecture:** One `rmserver.exe` with command-line modes. A `-service` process (LocalSystem, session 0) owns the TCP listener + HMAC auth + event parsing, then forwards each injection event over a named pipe to a `-agent` process that it launches onto the current input desktop (`Default` when unlocked, `Winlogon` when locked) via `CreateProcessAsUser`. The agent runs the existing `SendInput` injector. The default (no-flag) mode becomes a tray UI for status/config and service install/start/stop. The existing `Injector` interface is the reuse seam: the service injects through a `pipeInjector` that marshals each call to newline-JSON; the agent replays it through the real `winInjector`.

**Tech Stack:** Go 1.26, `golang.org/x/sys/windows` (`svc`, `svc/mgr`, `registry`), `github.com/Microsoft/go-winio` (named pipes), existing `injector_windows.go` (`SendInput`), existing PBKDF2/HMAC handshake.

**Source of truth:** `docs/superpowers/specs/2026-06-30-windows-lock-screen-design.md`.

**Conventions for every task below:**
- All Go commands run from the **`server/`** directory (that is where `go.mod` lives).
- The development/build target is **windows/amd64** (native on this machine). `build.ps1` sets `GOOS`/`GOARCH`.
- Every commit message ends with the trailer:
  `Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>`
- Win32 spawn/token/secure-desktop behavior **cannot be unit-tested**. Tasks extract pure logic for TDD and use a documented manual validation loop for the rest.

---

## File Structure

**New, shared (no build tag) — testable on any OS:**
- `server/inject_apply.go` — `applyEvent(inj Injector, m In)`: dispatch one pointer/keyboard event to an injector. Used by both the in-process server and the agent loop.
- `server/pipe_inject.go` — `pipeInjector` (an `Injector` that writes newline-JSON `In` to a swappable `io.Writer`) and `runAgentLoop(r io.Reader, inj Injector)` (read newline-JSON `In`, apply via `applyEvent`).
- `server/serve.go` — `serveStandalone(cfg appConfig)`, `announceMDNS(cfg, id)`, `newDevID()`: today's single-process behavior, extracted from `main.go`.

**New, Windows-only (`//go:build windows`):**
- `server/config_windows.go` — read/write `HKLM\SOFTWARE\RemoteMouse` (`Password`, `Port`); root is a parameter so tests use `HKCU`.
- `server/ipc_windows.go` — go-winio pipe listen/dial with a POC DACL.
- `server/desktop_windows.go` — `currentInputDesktop()`, `activeConsoleSession()`, `tokenStrategyFor(name)`, `probeDesktop()`.
- `server/spawn_windows.go` — privilege enabling, user/winlogon token acquisition, `findProcessInSession`, `spawnAgentOnDesktop` (`CreateProcessAsUser`).
- `server/service_windows.go` — svc `Handler`, `installService`/`uninstallService` via `mgr`, `runServiceCore`, `agentMonitor` (desktop-follow loop), file logger.
- `server/agent_windows.go` — `runAgent(cfg)`: dial the pipe, run `runAgentLoop` against the real injector.
- `server/run_windows.go` — `run(cfg)`: dispatch by `cfg.mode()`.

**New, non-Windows (`//go:build !windows`):**
- `server/run_other.go` — `run(cfg)` = `serveStandalone(cfg)` (the only mode off Windows).

**Modified:**
- `server/server.go` — `handle` loop calls `applyEvent`; `ping`/`bye` stay inline. Behavior identical.
- `server/main.go` — parse flags into `appConfig`; call `run(cfg)`. `appConfig` + `mode()` defined here.
- `server/ui_windows.go` — default mode UI gains service install/start/stop/status and config display.
- `docs/09-lock-screen.md`, `docs/12-progress.md` — progress + install instructions.

**Reused unchanged:** `server/proto.go` (`In`, `encode`), `server/injector_windows.go` (`winInjector`/`SendInput`), `server/clients.go` (`ClientRegistry`), `server/keys*.go`.

---

## Task 1: Extract `applyEvent` and decouple the server loop

**Files:**
- Create: `server/inject_apply.go`
- Create (test): `server/inject_apply_test.go`
- Modify: `server/server.go:77-101` (the event `switch` inside `handle`)

- [ ] **Step 1: Write the failing test**

Create `server/inject_apply_test.go`. This also defines `fakeInjector` and the `bptr` helper reused by later tasks' tests.

```go
package main

import (
	"fmt"
	"reflect"
	"testing"
)

// fakeInjector records each Injector call as a readable string. Shared across
// test files in package main.
type fakeInjector struct{ calls []string }

func (f *fakeInjector) MoveRel(dx, dy int)            { f.calls = append(f.calls, fmt.Sprintf("move %d %d", dx, dy)) }
func (f *fakeInjector) Button(b string, down bool)    { f.calls = append(f.calls, fmt.Sprintf("button %s %v", b, down)) }
func (f *fakeInjector) Scroll(dx, dy int)             { f.calls = append(f.calls, fmt.Sprintf("scroll %d %d", dx, dy)) }
func (f *fakeInjector) Text(s string)                 { f.calls = append(f.calls, "text "+s) }
func (f *fakeInjector) Key(code, mods int, down bool) { f.calls = append(f.calls, fmt.Sprintf("key %d %d %v", code, mods, down)) }
func (f *fakeInjector) Close()                        {}

// bptr returns a pointer to b, for the *bool fields on In.
func bptr(b bool) *bool { return &b }

func TestApplyEventClickExpandsToTwoButtons(t *testing.T) {
	f := &fakeInjector{}
	applyEvent(f, In{T: "click", B: "left"})
	want := []string{"button left true", "button left false"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("click calls = %v, want %v", f.calls, want)
	}
}

func TestApplyEventCoversAllPointerAndKeyTypes(t *testing.T) {
	f := &fakeInjector{}
	applyEvent(f, In{T: "move", Dx: 3, Dy: -4})
	applyEvent(f, In{T: "button", B: "right", Down: bptr(true)})
	applyEvent(f, In{T: "scroll", Dx: 0, Dy: 2})
	applyEvent(f, In{T: "text", S: "hi"})
	applyEvent(f, In{T: "key", Code: 65, Mods: 2, Down: bptr(false)})
	want := []string{"move 3 -4", "button right true", "scroll 0 2", "text hi", "key 65 2 false"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestApplyEvent -v`
Expected: build failure — `undefined: applyEvent`.

- [ ] **Step 3: Create `server/inject_apply.go`**

```go
package main

// applyEvent dispatches a single client event to an injector. It handles the
// pointer/keyboard event types only; control-plane messages (ping/bye) are the
// caller's responsibility. "click" expands to a press + release so that, when
// inj is a pipeInjector, the expansion happens on the service side and the
// agent only ever sees primitive button events. Shared by the in-process
// server (server.go) and the agent pipe loop (pipe_inject.go).
func applyEvent(inj Injector, m In) {
	switch m.T {
	case "move":
		inj.MoveRel(m.Dx, m.Dy)
	case "button":
		inj.Button(m.B, m.Down != nil && *m.Down)
	case "click":
		inj.Button(m.B, true)
		inj.Button(m.B, false)
	case "scroll":
		inj.Scroll(m.Dx, m.Dy)
	case "text":
		inj.Text(m.S)
	case "key":
		inj.Key(m.Code, m.Mods, m.Down != nil && *m.Down)
	}
}
```

- [ ] **Step 4: Rewrite the `handle` event loop in `server/server.go`**

Replace the loop currently at lines 77-101:

```go
	for r.Scan() {
		var m In
		if json.Unmarshal(r.Bytes(), &m) != nil {
			continue
		}
		switch m.T {
		case "ping":
			s.send(c, map[string]any{"t": "pong", "ts": m.Ts})
		case "bye":
			return
		default:
			applyEvent(s.inj, m)
		}
	}
	log.Printf("client disconnected: %s", addr)
```

- [ ] **Step 5: Run the full test suite to verify it passes**

Run: `go test ./...`
Expected: PASS (existing `TestHandleRegistersAndDeregisters` and the two new `applyEvent` tests).

- [ ] **Step 6: Vet**

Run: `go vet ./...`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add server/inject_apply.go server/inject_apply_test.go server/server.go
git commit -m "refactor: extract applyEvent and decouple server event loop" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 2: `pipeInjector` and `runAgentLoop`

**Files:**
- Create: `server/pipe_inject.go`
- Create (test): `server/pipe_inject_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/pipe_inject_test.go` (reuses `fakeInjector`/`bptr` from Task 1):

```go
package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestPipeInjectorRoundTripsThroughAgentLoop(t *testing.T) {
	// pipeInjector serialises calls as newline-JSON In events into a buffer;
	// runAgentLoop reads them back and drives a fakeInjector. Asserts events
	// survive the round trip, including click -> two buttons.
	var buf bytes.Buffer
	pi := newPipeInjector(&buf)
	applyEvent(pi, In{T: "move", Dx: 5, Dy: -7})
	applyEvent(pi, In{T: "click", B: "left"})
	applyEvent(pi, In{T: "text", S: "hi"})

	f := &fakeInjector{}
	runAgentLoop(&buf, f)

	want := []string{"move 5 -7", "button left true", "button left false", "text hi"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
}

func TestPipeInjectorSetWriterSwapsTarget(t *testing.T) {
	var a, b bytes.Buffer
	pi := newPipeInjector(&a)
	pi.MoveRel(1, 1)
	pi.setWriter(&b)
	pi.MoveRel(2, 2)
	if a.Len() == 0 || b.Len() == 0 {
		t.Fatalf("expected writes to both buffers, a=%d b=%d", a.Len(), b.Len())
	}
}

func TestPipeInjectorNilWriterIsNoop(t *testing.T) {
	pi := newPipeInjector(nil)
	pi.MoveRel(1, 1) // must not panic
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestPipeInjector -v`
Expected: build failure — `undefined: newPipeInjector` / `undefined: runAgentLoop`.

- [ ] **Step 3: Create `server/pipe_inject.go`**

```go
package main

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
)

// pipeInjector implements Injector by serialising each call as a newline-JSON
// In event onto a writer (the named pipe to the agent). The writer is swappable
// under a mutex so the service can rebind to a freshly spawned agent when the
// input desktop changes. The mutex is held across the write so concurrent
// client goroutines cannot interleave bytes on the pipe.
type pipeInjector struct {
	mu sync.Mutex
	w  io.Writer
}

func newPipeInjector(w io.Writer) *pipeInjector { return &pipeInjector{w: w} }

func (p *pipeInjector) setWriter(w io.Writer) {
	p.mu.Lock()
	p.w = w
	p.mu.Unlock()
}

func (p *pipeInjector) emit(m In) {
	b, err := encode(m)
	if err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.w == nil {
		return
	}
	p.w.Write(b)
}

func (p *pipeInjector) MoveRel(dx, dy int) { p.emit(In{T: "move", Dx: dx, Dy: dy}) }
func (p *pipeInjector) Button(b string, down bool) {
	d := down
	p.emit(In{T: "button", B: b, Down: &d})
}
func (p *pipeInjector) Scroll(dx, dy int) { p.emit(In{T: "scroll", Dx: dx, Dy: dy}) }
func (p *pipeInjector) Text(s string)     { p.emit(In{T: "text", S: s}) }
func (p *pipeInjector) Key(code, mods int, down bool) {
	d := down
	p.emit(In{T: "key", Code: code, Mods: mods, Down: &d})
}
func (p *pipeInjector) Close() {}

// runAgentLoop reads newline-delimited In events from r and applies each to inj
// until r is exhausted. The agent process uses it to drive SendInput from
// events the service forwards over the pipe.
func runAgentLoop(r io.Reader, inj Injector) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var m In
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		applyEvent(inj, m)
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run TestPipeInjector -v`
Expected: PASS (all three).

- [ ] **Step 5: Full suite + vet**

Run: `go test ./... && go vet ./...`
Expected: PASS, no vet output.

- [ ] **Step 6: Commit**

```bash
git add server/pipe_inject.go server/pipe_inject_test.go
git commit -m "feat: add pipeInjector and runAgentLoop for service/agent IPC" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 3: Run-mode dispatch and `serveStandalone` extraction

**Files:**
- Modify: `server/main.go` (full rewrite — add `appConfig`, `mode()`, mode flags, `run(cfg)`)
- Create (test): `server/main_test.go`
- Create: `server/serve.go`
- Create: `server/run_other.go`
- Create: `server/run_windows.go`

- [ ] **Step 1: Write the failing test for `mode()`**

Create `server/main_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestAppConfigMode -v`
Expected: build failure — `undefined: appConfig`.

- [ ] **Step 3: Rewrite `server/main.go`**

```go
package main

import (
	"flag"
	"os"
	"runtime"
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

// appConfig holds parsed CLI configuration and selects the run mode.
type appConfig struct {
	name   string
	pass   string
	port   int
	notray bool

	service          bool
	agent            bool
	pipe             string
	installService   bool
	uninstallService bool
	probeDesktop     bool
	standalone       bool
}

// mode resolves the run mode from the flags. install/uninstall win over service
// so an admin can (re)install without the SCM-launched copy interfering.
func (c appConfig) mode() string {
	switch {
	case c.installService:
		return "install-service"
	case c.uninstallService:
		return "uninstall-service"
	case c.service:
		return "service"
	case c.agent:
		return "agent"
	case c.probeDesktop:
		return "probe-desktop"
	case c.standalone:
		return "standalone"
	default:
		return "ui"
	}
}

func main() {
	host, _ := os.Hostname()
	var cfg appConfig
	flag.StringVar(&cfg.name, "name", sanitizeName(host), "device display name")
	flag.StringVar(&cfg.pass, "pass", "1234", "connection password")
	flag.IntVar(&cfg.port, "port", 27500, "TCP control port")
	flag.BoolVar(&cfg.notray, "notray", false, "console mode, no tray icon")
	flag.BoolVar(&cfg.service, "service", false, "run as the Windows service (LocalSystem)")
	flag.BoolVar(&cfg.agent, "agent", false, "run as the injection agent on the input desktop")
	flag.StringVar(&cfg.pipe, "pipe", `\\.\pipe\remotemouse-inject`, "named pipe for service<->agent IPC")
	flag.BoolVar(&cfg.installService, "install-service", false, "install the Windows service (run as admin)")
	flag.BoolVar(&cfg.uninstallService, "uninstall-service", false, "remove the Windows service (run as admin)")
	flag.BoolVar(&cfg.probeDesktop, "probe-desktop", false, "print the current input desktop in a loop (dev)")
	flag.BoolVar(&cfg.standalone, "standalone", false, "listen + inject in one process, Default desktop only (dev)")
	flag.Parse()

	run(cfg)
}
```

- [ ] **Step 4: Create `server/serve.go`**

```go
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
```

- [ ] **Step 5: Create `server/run_other.go`**

```go
//go:build !windows

package main

// run on non-Windows platforms only supports the standalone server.
func run(cfg appConfig) { serveStandalone(cfg) }
```

- [ ] **Step 6: Create `server/run_windows.go` (dispatch skeleton)**

```go
//go:build windows

package main

import "log"

// run dispatches by mode. The log.Fatalf branches are temporary runtime guards,
// replaced as each Windows mode lands in later tasks.
func run(cfg appConfig) {
	switch cfg.mode() {
	case "standalone":
		serveStandalone(cfg)
	case "probe-desktop":
		log.Fatalf("probe-desktop not yet implemented") // Task 6
	case "install-service":
		log.Fatalf("install-service not yet implemented") // Task 8
	case "uninstall-service":
		log.Fatalf("uninstall-service not yet implemented") // Task 8
	case "service":
		log.Fatalf("service not yet implemented") // Task 8
	case "agent":
		log.Fatalf("agent not yet implemented") // Task 8
	default: // "ui": today's listen+inject+tray; becomes UI-only in Task 9
		serveStandalone(cfg)
	}
}
```

- [ ] **Step 7: Build and test on Windows**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: build succeeds; all tests PASS; no vet output.

- [ ] **Step 8: Verify the non-Windows seam still compiles**

Run:
```powershell
$env:GOOS='linux'; $env:CGO_ENABLED='0'; go build ./...; $code=$LASTEXITCODE; Remove-Item Env:GOOS; Remove-Item Env:CGO_ENABLED; exit $code
```
Expected: exit 0. (Linux uses `run_other.go` + the stub injector/UI — pure Go. `run_other.go` is shared by all `!windows` platforms, so this also validates the darwin seam, which cannot be cgo-cross-built from Windows.)

- [ ] **Step 9: Commit**

```bash
git add server/main.go server/main_test.go server/serve.go server/run_other.go server/run_windows.go
git commit -m "feat: add run-mode dispatch and extract serveStandalone" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 4: HKLM configuration (`config_windows.go`)

**Files:**
- Create: `server/config_windows.go`
- Create (test): `server/config_windows_test.go`

The read/write functions take the registry **root** as a parameter: production passes `registry.LOCAL_MACHINE`; the test passes `registry.CURRENT_USER` so it needs no admin rights.

- [ ] **Step 1: Write the failing test**

Create `server/config_windows_test.go`:

```go
//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestConfigRoundTripUnderHKCU(t *testing.T) {
	// Use a throwaway HKCU key so the test needs no admin rights.
	root := registry.CURRENT_USER
	defer registry.DeleteKey(root, configPath)

	want := serverConfig{Password: "s3cret", Port: 28000}
	if err := writeConfig(root, want); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	got, err := readConfig(root)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestReadConfigMissingKeyReturnsDefaults(t *testing.T) {
	registry.DeleteKey(registry.CURRENT_USER, configPath) // ensure absent
	got, err := readConfig(registry.CURRENT_USER)
	if err == nil {
		t.Fatalf("expected error for missing key")
	}
	if got.Port != 27500 || got.Password != "1234" {
		t.Fatalf("defaults not returned: %+v", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run Config -v`
Expected: build failure — `undefined: serverConfig` / `undefined: readConfig`.

- [ ] **Step 3: Create `server/config_windows.go`**

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./... -run Config -v`
Expected: PASS (both).

- [ ] **Step 5: Full suite + vet**

Run: `go test ./... && go vet ./...`
Expected: PASS, no vet output.

- [ ] **Step 6: Commit**

```bash
git add server/config_windows.go server/config_windows_test.go
git commit -m "feat: add HKLM registry config read/write" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 5: Named-pipe IPC (`ipc_windows.go`)

**Files:**
- Create: `server/ipc_windows.go`
- Create (test): `server/ipc_windows_test.go`
- Modify: `server/go.mod`, `server/go.sum` (add `github.com/Microsoft/go-winio`)

- [ ] **Step 1: Write the failing test**

Create `server/ipc_windows_test.go`:

```go
//go:build windows

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInjectPipeRoundTrip(t *testing.T) {
	name := `\\.\pipe\remotemouse-test-` + strconv.Itoa(os.Getpid())
	ln, err := listenInjectPipe(name)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			got <- "accept-error: " + err.Error()
			return
		}
		defer c.Close()
		line, _ := bufio.NewReader(c).ReadString('\n')
		got <- strings.TrimSpace(line)
	}()

	c, err := dialInjectPipe(name)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if _, err := c.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case s := <-got:
		if s != "ping" {
			t.Fatalf("server read %q, want ping", s)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server read")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestInjectPipeRoundTrip -v`
Expected: build failure — `undefined: listenInjectPipe` / `undefined: dialInjectPipe`.

- [ ] **Step 3: Add the go-winio dependency**

Run:
```bash
go get github.com/Microsoft/go-winio
go mod tidy
```
Expected: `go.mod` now requires `github.com/Microsoft/go-winio`; `go.sum` updated.

- [ ] **Step 4: Create `server/ipc_windows.go`**

```go
//go:build windows

package main

import (
	"net"

	"github.com/Microsoft/go-winio"
)

// pipeSDDL is the POC DACL for the inject pipe: full access to SYSTEM (SY),
// the Administrators group (BA), and interactive users (IU). This is permissive
// for the POC and is tightened in a follow-up (spec section 15).
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)"

// listenInjectPipe creates the service (server) side of the inject pipe.
func listenInjectPipe(name string) (net.Listener, error) {
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: pipeSDDL})
}

// dialInjectPipe connects the agent (client) side to the inject pipe.
func dialInjectPipe(name string) (net.Conn, error) {
	return winio.DialPipe(name, nil)
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./... -run TestInjectPipeRoundTrip -v`
Expected: PASS.

- [ ] **Step 6: Full suite + vet**

Run: `go test ./... && go vet ./...`
Expected: PASS, no vet output.

- [ ] **Step 7: Commit**

```bash
git add server/ipc_windows.go server/ipc_windows_test.go server/go.mod server/go.sum
git commit -m "feat: add go-winio named-pipe IPC for inject channel" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 6: Input-desktop detection (`desktop_windows.go`)

**Files:**
- Create: `server/desktop_windows.go`
- Create (test): `server/desktop_windows_test.go`
- Modify: `server/run_windows.go` (wire the `probe-desktop` branch)

- [ ] **Step 1: Write the failing test**

Create `server/desktop_windows_test.go`:

```go
//go:build windows

package main

import "testing"

func TestTokenStrategyFor(t *testing.T) {
	cases := map[string]tokenStrategy{
		"Default":      tokenUser,
		"Winlogon":     tokenWinlogon,
		"Screen-saver": tokenWinlogon,
		"whatever":     tokenWinlogon,
	}
	for desk, want := range cases {
		if got := tokenStrategyFor(desk); got != want {
			t.Errorf("tokenStrategyFor(%q) = %v, want %v", desk, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestTokenStrategyFor -v`
Expected: build failure — `undefined: tokenStrategy` / `undefined: tokenStrategyFor`.

- [ ] **Step 3: Create `server/desktop_windows.go`**

```go
//go:build windows

package main

import (
	"log"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32                    = windows.NewLazySystemDLL("user32.dll")
	procOpenInputDesktop         = modUser32.NewProc("OpenInputDesktop")
	procGetUserObjectInformation = modUser32.NewProc("GetUserObjectInformationW")
	procCloseDesktop             = modUser32.NewProc("CloseDesktop")
)

const uoiName = 2 // UOI_NAME

// currentInputDesktop returns the name of the desktop currently receiving user
// input: "Default" when unlocked, "Winlogon" on the lock/login secure desktop,
// or "Screen-saver". Reading the name needs no special privilege, but a normal
// user-session process may be denied OpenInputDesktop while a secure desktop is
// active — the SYSTEM service is the authoritative caller.
func currentInputDesktop() (string, error) {
	hd, _, err := procOpenInputDesktop.Call(0, 0, uintptr(windows.GENERIC_READ))
	if hd == 0 {
		return "", err
	}
	defer procCloseDesktop.Call(hd)

	var buf [256]uint16
	var needed uint32
	r, _, err := procGetUserObjectInformation.Call(
		hd, uintptr(uoiName),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)*2),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return "", err
	}
	return syscall.UTF16ToString(buf[:]), nil
}

// activeConsoleSession returns the session attached to the physical console.
func activeConsoleSession() uint32 { return windows.WTSGetActiveConsoleSessionId() }

type tokenStrategy int

const (
	tokenUser     tokenStrategy = iota // interactive user token (Default desktop)
	tokenWinlogon                      // winlogon SYSTEM token (secure desktop)
)

// tokenStrategyFor maps an input-desktop name to the token strategy the service
// must use to spawn an agent that can inject on that desktop. Anything that is
// not the unlocked "Default" desktop is treated as secure.
func tokenStrategyFor(desktop string) tokenStrategy {
	if desktop == "Default" {
		return tokenUser
	}
	return tokenWinlogon
}

// probeDesktop prints the current input desktop in a loop (the -probe-desktop
// dev mode). Run it, then Win+L and unlock, and read the console scrollback to
// confirm the Default<->Winlogon transitions.
func probeDesktop() {
	for {
		if name, err := currentInputDesktop(); err != nil {
			log.Printf("probe: OpenInputDesktop failed: %v (likely a secure desktop we can't read)", err)
		} else {
			log.Printf("probe: input desktop=%q console session=%d", name, activeConsoleSession())
		}
		time.Sleep(500 * time.Millisecond)
	}
}
```

- [ ] **Step 4: Wire the `probe-desktop` branch in `server/run_windows.go`**

Replace:
```go
	case "probe-desktop":
		log.Fatalf("probe-desktop not yet implemented") // Task 6
```
with:
```go
	case "probe-desktop":
		probeDesktop()
```

- [ ] **Step 5: Run the test + full suite + vet**

Run: `go test ./... && go vet ./...`
Expected: PASS, no vet output.

- [ ] **Step 6: Manual validation (dev aid)**

Run from a normal console: `go run . -probe-desktop`
Expected output: `input desktop="Default" console session=1` repeating. Press `Win+L`; either the line reports `"Winlogon"` or logs an `OpenInputDesktop failed` access-denied (both confirm the secure-desktop switch from a non-elevated probe). Unlock; it returns to `"Default"`. Ctrl+C to stop.

- [ ] **Step 7: Commit**

```bash
git add server/desktop_windows.go server/desktop_windows_test.go server/run_windows.go
git commit -m "feat: detect current input desktop and token strategy" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 7: Token acquisition + agent spawn (`spawn_windows.go`)

These call Win32 token/`CreateProcessAsUser` APIs that cannot be unit-tested without a service context and a real desktop. Only `findProcessInSession` is testable on its own (find the running test binary in its own session); the rest are gated by `go build`/`go vet` here and validated at runtime in Task 8.

**Files:**
- Create: `server/spawn_windows.go`
- Create (test): `server/spawn_windows_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/spawn_windows_test.go`:

```go
//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestFindProcessInSessionFindsSelf(t *testing.T) {
	// The running test binary must be findable in its own session.
	var sess uint32
	r, _, _ := procProcessIdToSessId.Call(
		uintptr(windows.GetCurrentProcessId()), uintptr(unsafe.Pointer(&sess)))
	if r == 0 {
		t.Skip("ProcessIdToSessionId unavailable")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	name := filepath.Base(exe)
	pid, err := findProcessInSession(name, sess)
	if err != nil {
		t.Fatalf("findProcessInSession(%q, %d): %v", name, sess, err)
	}
	if pid == 0 {
		t.Fatalf("pid = 0")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestFindProcessInSessionFindsSelf -v`
Expected: build failure — `undefined: procProcessIdToSessId` / `undefined: findProcessInSession`.

- [ ] **Step 3: Create `server/spawn_windows.go`**

```go
//go:build windows

package main

import (
	"fmt"
	"log"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procProcessIdToSessId = modKernel32.NewProc("ProcessIdToSessionId")
)

// enablePrivilege enables a named privilege on the current process token.
func enablePrivilege(name string) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()

	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(name), &luid); err != nil {
		return fmt.Errorf("LookupPrivilegeValue %s: %w", name, err)
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}

// enableServicePrivileges enables the privileges LocalSystem needs to query a
// user token and launch a process as that user on another desktop.
func enableServicePrivileges() {
	for _, p := range []string{"SeTcbPrivilege", "SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege"} {
		if err := enablePrivilege(p); err != nil {
			log.Printf("enablePrivilege %s: %v", p, err)
		}
	}
}

// userTokenForSession returns a primary token for the interactive user in the
// session (used when the input desktop is Default / unlocked).
func userTokenForSession(session uint32) (windows.Token, error) {
	var tok windows.Token
	if err := windows.WTSQueryUserToken(session, &tok); err != nil {
		return 0, fmt.Errorf("WTSQueryUserToken(%d): %w", session, err)
	}
	defer tok.Close()
	return duplicatePrimary(tok)
}

// winlogonTokenForSession returns a primary token cloned from winlogon.exe in
// the session (a SYSTEM token that can reach the secure desktop).
func winlogonTokenForSession(session uint32) (windows.Token, error) {
	pid, err := findProcessInSession("winlogon.exe", session)
	if err != nil {
		return 0, err
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, pid)
	if err != nil {
		return 0, fmt.Errorf("OpenProcess winlogon pid=%d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	var tok windows.Token
	if err := windows.OpenProcessToken(h,
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY, &tok); err != nil {
		return 0, fmt.Errorf("OpenProcessToken winlogon: %w", err)
	}
	defer tok.Close()
	return duplicatePrimary(tok)
}

func duplicatePrimary(tok windows.Token) (windows.Token, error) {
	var dup windows.Token
	if err := windows.DuplicateTokenEx(tok, windows.MAXIMUM_ALLOWED, nil,
		windows.SecurityImpersonation, windows.TokenPrimary, &dup); err != nil {
		return 0, fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	return dup, nil
}

// findProcessInSession returns the PID of the first process named exe (case-
// insensitive) running in session, via a Toolhelp snapshot + ProcessIdToSessionId.
func findProcessInSession(exe string, session uint32) (uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), exe) {
			continue
		}
		var sess uint32
		r, _, _ := procProcessIdToSessId.Call(uintptr(pe.ProcessID), uintptr(unsafe.Pointer(&sess)))
		if r != 0 && sess == session {
			return pe.ProcessID, nil
		}
	}
	return 0, fmt.Errorf("%s not found in session %d", exe, session)
}

// spawnAgentOnDesktop launches exePath as the given token, bound to
// WinSta0\<desktop>, telling it to connect to the inject pipe. It returns the
// process handle so the caller can TerminateProcess it on the next desktop
// switch or on shutdown.
func spawnAgentOnDesktop(token windows.Token, exePath, desktop, pipe string) (windows.Handle, error) {
	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, token, false); err != nil {
		return 0, fmt.Errorf("CreateEnvironmentBlock: %w", err)
	}
	defer windows.DestroyEnvironmentBlock(env)

	deskPath := `WinSta0\` + desktop
	cmd := fmt.Sprintf(`"%s" -agent -pipe %s`, exePath, pipe)

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = windows.StringToUTF16Ptr(deskPath)

	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(token, nil, windows.StringToUTF16Ptr(cmd),
		nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW,
		env, nil, &si, &pi); err != nil {
		return 0, fmt.Errorf("CreateProcessAsUser desktop=%s: %w", deskPath, err)
	}
	windows.CloseHandle(pi.Thread)
	return pi.Process, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestFindProcessInSessionFindsSelf -v`
Expected: PASS.

- [ ] **Step 5: Build + full suite + vet (gates the un-testable funcs)**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: build succeeds; tests PASS; no vet output.

- [ ] **Step 6: Commit**

```bash
git add server/spawn_windows.go server/spawn_windows_test.go
git commit -m "feat: add token acquisition and CreateProcessAsUser agent spawn" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 8: Service, agent, and desktop-follow monitor

This wires everything into a working service. The Win32 spawn/token/secure-desktop behavior cannot be unit-tested, so the gate here is `go build`/`go test`/`go vet` (all green) plus the documented manual end-to-end loop. Commit after the static gates pass; if manual validation surfaces issues, fix and commit again.

**Files:**
- Create: `server/agent_windows.go`
- Create: `server/service_windows.go`
- Modify: `server/run_windows.go` (wire the `service`, `agent`, `install-service`, `uninstall-service` branches)

- [ ] **Step 1: Create `server/agent_windows.go`**

```go
//go:build windows

package main

import (
	"log"
	"net"
	"time"
)

// runAgent connects to the service's inject pipe and applies forwarded events
// via SendInput on whatever desktop this process was spawned onto. The service
// terminates this process when the input desktop changes, so on pipe close the
// agent simply exits.
func runAgent(cfg appConfig) {
	setupFileLogger("agent")
	log.Printf("agent starting, pipe=%s", cfg.pipe)

	var conn net.Conn
	for i := 0; i < 20; i++ { // ~5s of startup retries while the listener comes up
		c, err := dialInjectPipe(cfg.pipe)
		if err == nil {
			conn = c
			break
		}
		log.Printf("agent dial failed (attempt %d): %v", i+1, err)
		time.Sleep(250 * time.Millisecond)
	}
	if conn == nil {
		log.Printf("agent giving up: could not connect to %s", cfg.pipe)
		return
	}
	defer conn.Close()
	log.Printf("agent connected; injecting on this desktop")

	inj := newInjector() // winInjector (real SendInput)
	defer inj.Close()
	runAgentLoop(conn, inj)
	log.Printf("agent pipe closed; exiting")
}
```

- [ ] **Step 2: Create `server/service_windows.go`**

```go
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

	m.pi.setWriter(nil)
	if conn != nil {
		conn.Close()
	}
	if h != 0 {
		windows.TerminateProcess(h, 0)
		windows.CloseHandle(h)
	}
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
```

- [ ] **Step 3: Wire the remaining branches in `server/run_windows.go`**

Replace these four branches:
```go
	case "install-service":
		log.Fatalf("install-service not yet implemented") // Task 8
	case "uninstall-service":
		log.Fatalf("uninstall-service not yet implemented") // Task 8
	case "service":
		log.Fatalf("service not yet implemented") // Task 8
	case "agent":
		log.Fatalf("agent not yet implemented") // Task 8
```
with:
```go
	case "install-service":
		if err := installService(cfg); err != nil {
			log.Fatalf("install-service: %v", err)
		}
	case "uninstall-service":
		if err := uninstallService(); err != nil {
			log.Fatalf("uninstall-service: %v", err)
		}
	case "service":
		runService(cfg)
	case "agent":
		runAgent(cfg)
```

- [ ] **Step 4: Static gates — build, test, vet**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: build succeeds; all tests PASS; no vet output.

- [ ] **Step 5: Commit**

```bash
git add server/agent_windows.go server/service_windows.go server/run_windows.go
git commit -m "feat: add LocalSystem service, agent, and desktop-follow monitor" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

- [ ] **Step 6: Build the release exe and install the service (admin)**

Build: `./build.ps1` (or `go build -o rmserver.exe .` from `server/`).
In an **elevated** PowerShell, from the build output directory:
```powershell
.\rmserver.exe -install-service -pass 1234 -port 27500
sc.exe start RemoteMouse
sc.exe query RemoteMouse   # expect STATE: RUNNING
```
Expected: `installed service "RemoteMouse" ...`; service reaches RUNNING. Check `C:\ProgramData\RemoteMouse\service.log` shows `service log opened`, `config: port=27500`, and `agent bound to session=N desktop="Default"`. Check `agent.log` shows `agent connected`.

- [ ] **Step 7: Validate the unlocked path first (isolate the lock-screen variable)**

With the screen **unlocked**, connect from the iPhone (password `1234`) and move/click/scroll/type.
Expected: cursor and keyboard respond exactly as before — proving the service→pipe→agent→SendInput chain works on `Default` before lock-screen is involved.

- [ ] **Step 8: Validate the lock-screen path end to end**

Press `Win+L`. Watch `service.log` for `desktop change -> ... desktop="Winlogon"` then `agent bound to ... desktop="Winlogon"`. On the iPhone, type the Windows Hello **PIN** and press Enter.
Expected: the PIN appears in the lock-screen field and the PC unlocks.

- [ ] **Step 9: Validate rebind after unlock**

After unlocking, watch `service.log` for `desktop change -> ... desktop="Default"` and confirm the iPhone controls the mouse/keyboard again without any manual step.
Expected: injection follows back to `Default` automatically.

- [ ] **Step 10: Record results / fix-forward**

If steps 7-9 pass, Task 8 is done. If any step fails, read `service.log`/`agent.log`, fix the cause (common culprits: a missing privilege in `enableServicePrivileges`, the wrong desktop path in `spawnAgentOnDesktop`, or the agent failing to dial), commit the fix with a descriptive message + the Co-authored-by trailer, and re-run steps 6-9.

To remove the service when finished testing:
```powershell
.\rmserver.exe -uninstall-service   # elevated
```

---

## Task 9: Tray UI service control (`ui_service_windows.go`)

The default (no-flag) mode becomes a status/config UI that installs and controls the service instead of listening/injecting itself. No automated test (pure UI/Win32); gate is `go build`/`go vet` + manual check.

**Files:**
- Create: `server/ui_service_windows.go`
- Modify: `server/ui_windows.go` (add service actions to the tray menu)
- Modify: `server/service_windows.go` (`installService` also starts the service)
- Modify: `server/run_windows.go` (default branch → `serveUI`)

- [ ] **Step 1: Create `server/ui_service_windows.go`**

```go
//go:build windows

package main

import (
	"log"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// serveUI is the default (no-flag) mode: a tray UI that shows connection info
// from HKLM and installs/controls the service. It does NOT listen on TCP or
// inject — the service owns that. The device list is empty here (the service
// owns the real connections); a status pipe to populate it is a POC follow-up.
func serveUI(cfg appConfig) {
	if sc, err := readConfig(registry.LOCAL_MACHINE); err == nil {
		cfg.pass, cfg.port = sc.Password, sc.Port
	}
	ips := DetectIPs()
	reg := NewClientRegistry()
	runUI(cfg.notray, cfg.pass, cfg.port, ips, reg)
}

// elevatedSelf relaunches this executable elevated (UAC) with args, used for
// install/uninstall which require admin.
func elevatedSelf(args ...string) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(selfPath())
	argp, _ := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err := windows.ShellExecute(0, verb, file, argp, nil, windows.SW_NORMAL); err != nil {
		log.Printf("elevatedSelf %v: %v", args, err)
	}
}

// serviceState returns a human-readable service status using read-only SCM
// access (no admin required).
func serviceState() string {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return "未知"
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString(svcName)
	sh, err := windows.OpenService(scm, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return "未安装"
	}
	defer windows.CloseServiceHandle(sh)
	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(sh, &st); err != nil {
		return "未知"
	}
	switch st.CurrentState {
	case windows.SERVICE_RUNNING:
		return "运行中"
	case windows.SERVICE_STOPPED:
		return "已停止"
	case windows.SERVICE_START_PENDING:
		return "启动中"
	case windows.SERVICE_STOP_PENDING:
		return "停止中"
	default:
		return "未知"
	}
}
```

- [ ] **Step 2: Add service actions to the tray menu in `server/ui_windows.go`**

Find:
```go
		ni.ContextMenu().Actions().Add(autoAct)

		ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())
```
Replace with:
```go
		ni.ContextMenu().Actions().Add(autoAct)

		// Service control: lock-screen-capable injection runs in the SYSTEM service.
		svcStatus := walk.NewAction()
		svcStatus.SetText("服务状态：" + serviceState())
		svcStatus.SetEnabled(false)
		ni.ContextMenu().Actions().Add(svcStatus)

		svcInstall := walk.NewAction()
		svcInstall.SetText("安装并启动服务")
		svcInstall.Triggered().Attach(func() {
			elevatedSelf("-install-service", "-pass", pass, "-port", strconv.Itoa(port))
		})
		ni.ContextMenu().Actions().Add(svcInstall)

		svcUninstall := walk.NewAction()
		svcUninstall.SetText("卸载服务")
		svcUninstall.Triggered().Attach(func() { elevatedSelf("-uninstall-service") })
		ni.ContextMenu().Actions().Add(svcUninstall)

		ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())
```

- [ ] **Step 3: Make `installService` start the service (in `server/service_windows.go`)**

Find, inside `installService`:
```go
	defer s.Close()
	fmt.Printf("installed service %q -> %s -service (port %d)\n", svcName, exepath, cfg.port)
	return nil
```
Replace with:
```go
	defer s.Close()
	if err := s.Start(); err != nil {
		log.Printf("service created but start failed: %v", err)
	}
	fmt.Printf("installed service %q -> %s -service (port %d)\n", svcName, exepath, cfg.port)
	return nil
```

- [ ] **Step 4: Point the default mode at `serveUI` (in `server/run_windows.go`)**

Find:
```go
	default: // "ui": today's listen+inject+tray; becomes UI-only in Task 9
		serveStandalone(cfg)
```
Replace with:
```go
	default: // "ui"
		serveUI(cfg)
```

- [ ] **Step 5: Static gates — build, test, vet**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: build succeeds; all tests PASS; no vet output.

- [ ] **Step 6: Commit**

```bash
git add server/ui_service_windows.go server/ui_windows.go server/service_windows.go server/run_windows.go
git commit -m "feat: add service install/control to the tray UI" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

- [ ] **Step 7: Manual validation**

Build (`./build.ps1`) and run `rmserver.exe` with no flags. Expected: tray icon + window show IP/port/password; the tray menu shows `服务状态：未安装` (or `已停止`/`运行中`). Click `安装并启动服务` → UAC prompt → accept → service installs and starts (verify `sc.exe query RemoteMouse` = RUNNING). Re-open the app to see `服务状态：运行中`. `卸载服务` → UAC → service removed.

---

## Task 10: Documentation

**Files:**
- Modify: `docs/09-lock-screen.md`
- Modify: `docs/12-progress.md`

- [ ] **Step 1: Update `docs/09-lock-screen.md`**

Find the line:
```
- 风险：实现复杂、需签名、不同 Win 版本差异。MVP 后做。
```
Insert immediately after it:

````markdown
### 已实现（POC，2026-06-30）
单一 `rmserver.exe` 三模式：默认托盘 UI / `-service`（LocalSystem，持 TCP+鉴权+事件解析+桌面监控）/ `-agent`（被服务用 `CreateProcessAsUser` 拉到当前输入桌面，复用 `SendInput`）。服务经命名管道把事件转发给 agent；锁屏时检测到 `Winlogon` 桌面即用 winlogon token 在安全桌面重开 agent，从 iPhone 输入 Hello PIN 解锁。设计与计划见 `docs/superpowers/specs/2026-06-30-windows-lock-screen-design.md`、`docs/superpowers/plans/2026-06-30-windows-lock-screen.md`。

安装（管理员 PowerShell）：

```powershell
.\rmserver.exe -install-service -pass <密码> -port 27500   # 安装并启动
.\rmserver.exe -uninstall-service                          # 停止并卸载
```

日志：`C:\ProgramData\RemoteMouse\service.log`、`agent.log`（按键内容脱敏，不记录 PIN）。
仍是 POC：PIN 走明文 LAN，仅限可信网络；TLS 为紧随其后的下一步。
````

- [ ] **Step 2: Update the status line in `docs/12-progress.md`**

Find:
```
**更新时间**：2026-06-30 ｜ **状态**：M1+M2 完成；M3 部分完成（Windows server 真机注入 + 键盘/快捷/媒体映射 + 可视化窗口/托盘 UI + 自启，iOS 按键栏）
```
Replace with:
```
**更新时间**：2026-06-30 ｜ **状态**：M1+M2 完成；M3 部分完成（Windows server 真机注入 + 键盘/快捷/媒体映射 + 可视化窗口/托盘 UI + 自启，iOS 按键栏）；M5 锁屏注入 POC（Windows SYSTEM 服务 + 按桌面 agent，iPhone 打 Hello PIN 解锁）
```

- [ ] **Step 3: Add an 已实现 bullet in `docs/12-progress.md`**

Find the `- **iOS** ` bullet under `## 已实现` and insert this bullet immediately after it:
```
- **锁屏注入（M5 POC）** `server/`（Windows）：单 `rmserver.exe` 三模式——默认托盘 UI（读 HKLM 配置 + 安装/启停服务）、`-service`（LocalSystem 持 TCP+鉴权+事件解析，命名管道转发，桌面监控按 `Default`/`Winlogon` 跟随重开 agent）、`-agent`（`CreateProcessAsUser` 拉到当前输入桌面，复用 `SendInput`）。锁屏从 iPhone 输入 Hello PIN 解锁。日志写 `%ProgramData%\RemoteMouse`。
```

- [ ] **Step 4: Update the 下一步 list in `docs/12-progress.md`**

Find:
```
4. 锁屏（Win SYSTEM 服务）。
```
Replace with:
```
4. 锁屏（Win SYSTEM 服务）：POC 已通（install/start → 解锁态注入 → Win+L 检测 Winlogon → 打 PIN 解锁）。收尾：TLS（PIN 明文 LAN 风险）、WTS 通知替代轮询、管道/HKLM ACL 收紧、服务签名+安装包。
```

- [ ] **Step 5: Commit**

```bash
git add docs/09-lock-screen.md docs/12-progress.md
git commit -m "docs: record Windows lock-screen POC and install steps" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Self-Review (completed by plan author)

**1. Spec coverage** — every section maps to a task:
- §3/§5 three-mode binary → T3 (`mode()` + dispatch), wired across T6/T8/T9.
- §6 data flow / unified inject path → T1 (`applyEvent`) + T2 (`pipeInjector`/`runAgentLoop`) + T8 (service builds `Server` with `pipeInjector`).
- §7 desktop follow + agent lifecycle → T6 (`currentInputDesktop`) + T8 (`agentMonitor`).
- §8 token acquisition (Default vs Winlogon) → T6 (`tokenStrategyFor`) + T7 (`userTokenForSession`/`winlogonTokenForSession`/`enableServicePrivileges`).
- §9 `CreateProcessAsUser` → T7 (`spawnAgentOnDesktop`).
- §10 IPC pipe + frame format → T5 (`ipc_windows.go`) + T2 (newline-JSON `In` frames).
- §5/§11 config in HKLM → T4; service reads it in T8; UI reads it in T9.
- §12 file logging + `-probe-desktop` + manual loop → T6 (probe) + T8 (`setupFileLogger`, manual loop).
- §13 acceptance (PIN unlock, unlocked control, auto-follow) → T8 steps 7-9.
- §14 file list → matches the File Structure map.
- §15 follow-ups (TLS etc.) → recorded in T10 docs, intentionally out of scope.

**2. Placeholder scan** — no TBD/TODO/"handle errors"/"similar to Task N"; every code step shows complete code; the `log.Fatalf("... not yet implemented")` lines in T3 are intentional, compiling, staged runtime guards explicitly replaced in T6/T8.

**3. Type/name consistency** — verified against the installed `golang.org/x/sys/windows`: `applyEvent`, `pipeInjector`/`newPipeInjector`/`setWriter`/`emit`, `runAgentLoop`, `appConfig.mode()`, `serveStandalone`/`announceMDNS`/`newDevID`, `serverConfig`/`readConfig`/`writeConfig`/`configPath`, `listenInjectPipe`/`dialInjectPipe`/`pipeSDDL`, `currentInputDesktop`/`activeConsoleSession`/`tokenStrategyFor`/`tokenStrategy`(`tokenUser`/`tokenWinlogon`)/`probeDesktop`, `enablePrivilege`/`enableServicePrivileges`/`userTokenForSession`/`winlogonTokenForSession`/`duplicatePrimary`/`findProcessInSession`/`spawnAgentOnDesktop`/`procProcessIdToSessId`, `svcName`/`setupFileLogger`/`selfPath`/`runService`/`serviceHandler`/`runServiceCore`/`agentMonitor`/`installService`/`uninstallService`, `runAgent`, `serveUI`/`elevatedSelf`/`serviceState`. The `Injector` interface and `In` struct fields match `server/injector.go` and `server/proto.go`.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-30-windows-lock-screen.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. Tasks 8-9 need real-machine manual validation (install service, lock screen, type PIN), which the human must run.

**2. Inline Execution** — execute tasks in this session with checkpoints for review.

**Pushing:** the Part-1 injector fix is already pushed; the spec commit `ad5e17d` and every commit produced by this plan are **not** pushed. Pushing is a separate, explicit decision — confirm before pushing.
