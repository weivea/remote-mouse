# rmserver 控制面板 Implementation Plan

> **Status: ✅ 完成（2026-07-02）** — Task 1–12 已实现并提交；Task 13 自动化验证 `go vet` / `go build` / `go test ./...` 全通过（`-race` 因本机无 CGO/gcc 未跑），端到端手工冒烟已人工验证通过。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `rmserver.exe` 的主窗口升级为完整控制面板：安装/卸载/启停服务、设服务启动类型、实时显示服务与连接状态、可编辑并热应用端口/密码/设备名，并让服务模式下也能看到在线设备。

**Architecture:** UI 进程内三个协作单元——StatusTicker（每秒查 SCM 状态+启动类型，服务模式经命名管道拉设备快照）、ServingArbiter（按服务是否运行动态释放/接管 TCP 端口）、servingController（封装独立模式的 Server+Listener+mDNS，支持热重配）。所有需要管理员的操作沿用 `elevatedSelf` 以 UAC 提权拉起一次性子进程；UI 本身非管理员。

**Tech Stack:** Go 1.26；`github.com/lxn/walk`（GUI）；`github.com/Microsoft/go-winio`（命名管道）；`golang.org/x/sys/windows`、`.../svc`、`.../svc/mgr`、`.../registry`；`github.com/libp2p/zeroconf/v2`（mDNS）。

**规范来源:** `docs/superpowers/specs/2026-07-01-control-panel-design.md`

**约定:**
- 全部命令在 `F:\github-copilot\remote-mouse\server` 目录下执行（PowerShell）。
- Windows 专属文件带 `//go:build windows`；测试同理。开发机即 Windows，`go test ./...` 会编译并运行 windows-tagged 测试。
- 提交信息统一附带 `Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>`。

---

## 文件结构

| 文件 | 职责 | 变更 |
|------|------|------|
| `server/config_windows.go` | HKLM 连接配置读写 | 加 `Name` 字段 |
| `server/config_windows_test.go` | 配置往返测试 | 覆盖 `Name` |
| `server/starttype_windows.go` **(新)** | 启动类型字符串/常量/标签/下拉序号 纯映射 | 新建 |
| `server/starttype_windows_test.go` **(新)** | 映射单测 | 新建 |
| `server/status.go` **(新, 跨平台)** | 设备快照 JSON 编解码（纯函数） | 新建 |
| `server/status_test.go` **(新, 跨平台)** | 编解码往返单测 | 新建 |
| `server/status_windows.go` **(新)** | 状态命名管道 服务侧/UI 侧 I/O | 新建 |
| `server/serving_windows.go` **(新)** | ServingArbiter 纯决策 + 配置 diff + servingController | 新建 |
| `server/serving_windows_test.go` **(新)** | 决策/diff + controller 集成测试 | 新建 |
| `server/server.go` | TCP 服务核心 | `Serve(ln)` 拆分；`SetPassword/SetName`+getter 加锁 |
| `server/server_test.go` | 服务核心测试 | 加密码热换测试 |
| `server/service_windows.go` | 服务安装与核心 | 加 start/stop/restart/setStartType/applyConfigElevated；`runServiceCore` 起状态管道 |
| `server/ui_service_windows.go` | 默认模式入口 + 提权 + 状态查询 | 重写 `serveUI`；加 `serviceStartType()` |
| `server/ui_panel_windows.go` **(新)** | 控制面板窗口 `runControlPanel` | 新建 |
| `server/ui_windows.go` | 旧简易窗口（`-standalone` 用）+ 共享 helper | 基本不动（仅托盘自启项改名，见 T11） |
| `server/autostart_windows.go` | HKCU 登录自启 | Run 命令加 `-name`；提供改写辅助 |
| `server/main.go` | flag 解析 + mode | 加 4 个 flag/字段 + mode 分支 |
| `server/main_test.go` | mode 测试 | 加新 mode 用例 |
| `server/run_windows.go` | mode 派发 | 加新 mode 分支 |
| `server/README.md` | 文档 | 更新控制面板用法 |

---

## Task 1: `serverConfig` 增加 `Name`

**Files:**
- Modify: `server/config_windows.go`
- Test: `server/config_windows_test.go`

- [ ] **Step 1: 更新测试以覆盖 Name**

把 `server/config_windows_test.go` 的 `TestConfigRoundTripUnderHKCU` 改为：

```go
func TestConfigRoundTripUnderHKCU(t *testing.T) {
	// Use a throwaway HKCU key so the test needs no admin rights.
	root := registry.CURRENT_USER
	defer registry.DeleteKey(root, configPath)

	want := serverConfig{Password: "s3cret", Port: 28000, Name: "客厅PC"}
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test -run TestConfigRoundTripUnderHKCU ./...`
Expected: FAIL（编译错误：`unknown field 'Name' in struct literal`）

- [ ] **Step 3: 给 `serverConfig` 加 Name 并读写**

在 `server/config_windows.go` 中：结构体加字段——

```go
type serverConfig struct {
	Password string
	Port     int
	Name     string
}
```

`readConfig` 在读 `Port` 之后、`return` 之前加入：

```go
	if s, _, err := k.GetStringValue("Name"); err == nil {
		cfg.Name = s
	}
```

`writeConfig` 在 `SetStringValue("Password", ...)` 之后加入（放在 `SetDWordValue("Port", ...)` 之前）：

```go
	if err := k.SetStringValue("Name", cfg.Name); err != nil {
		return err
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test -run 'TestConfigRoundTrip|TestReadConfigMissing' ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 5: 提交**

```powershell
git add server/config_windows.go server/config_windows_test.go
git commit -m "feat(config): persist device Name in HKLM serverConfig" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 2: 启动类型纯映射

**Files:**
- Create: `server/starttype_windows.go`
- Test: `server/starttype_windows_test.go`

- [ ] **Step 1: 写失败测试**

Create `server/starttype_windows_test.go`:

```go
//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestStartTypeFromArg(t *testing.T) {
	cases := map[string]struct {
		val uint32
		ok  bool
	}{
		"auto":     {windows.SERVICE_AUTO_START, true},
		"manual":   {windows.SERVICE_DEMAND_START, true},
		"disabled": {windows.SERVICE_DISABLED, true},
		"bogus":    {0, false},
	}
	for arg, want := range cases {
		got, ok := startTypeFromArg(arg)
		if got != want.val || ok != want.ok {
			t.Errorf("startTypeFromArg(%q) = (%d,%v), want (%d,%v)", arg, got, ok, want.val, want.ok)
		}
	}
}

func TestStartTypeArgRoundTrip(t *testing.T) {
	for _, arg := range []string{"auto", "manual", "disabled"} {
		v, _ := startTypeFromArg(arg)
		if back := startTypeArg(v); back != arg {
			t.Errorf("startTypeArg(fromArg(%q)) = %q", arg, back)
		}
	}
}

func TestStartTypeIndexRoundTrip(t *testing.T) {
	for i := 0; i < 3; i++ {
		if got := startTypeIndex(startTypeByIndex(i)); got != i {
			t.Errorf("index round-trip i=%d got %d", i, got)
		}
	}
}

func TestStartTypeLabel(t *testing.T) {
	if startTypeLabel(windows.SERVICE_AUTO_START) != "自动" {
		t.Errorf("auto label wrong")
	}
	if startTypeLabel(9999) != "未知" {
		t.Errorf("unknown label wrong")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test -run TestStartType ./...`
Expected: FAIL（`undefined: startTypeFromArg` 等）

- [ ] **Step 3: 实现映射**

Create `server/starttype_windows.go`:

```go
//go:build windows

package main

import "golang.org/x/sys/windows"

// startTypeFromArg maps a CLI arg ("auto"/"manual"/"disabled") to the Win32
// service start type. ok is false for unrecognized input.
func startTypeFromArg(s string) (uint32, bool) {
	switch s {
	case "auto":
		return windows.SERVICE_AUTO_START, true
	case "manual":
		return windows.SERVICE_DEMAND_START, true
	case "disabled":
		return windows.SERVICE_DISABLED, true
	default:
		return 0, false
	}
}

// startTypeArg is the inverse of startTypeFromArg; returns "" for unknown.
func startTypeArg(t uint32) string {
	switch t {
	case windows.SERVICE_AUTO_START:
		return "auto"
	case windows.SERVICE_DEMAND_START:
		return "manual"
	case windows.SERVICE_DISABLED:
		return "disabled"
	default:
		return ""
	}
}

// startTypeLabel is the Chinese label shown in the combo box.
func startTypeLabel(t uint32) string {
	switch t {
	case windows.SERVICE_AUTO_START:
		return "自动"
	case windows.SERVICE_DEMAND_START:
		return "手动"
	case windows.SERVICE_DISABLED:
		return "禁用"
	default:
		return "未知"
	}
}

// startTypeCombo is the fixed combo-box order.
var startTypeCombo = []uint32{
	windows.SERVICE_AUTO_START,
	windows.SERVICE_DEMAND_START,
	windows.SERVICE_DISABLED,
}

// startTypeIndex returns the combo-box row for a start type, or -1.
func startTypeIndex(t uint32) int {
	for i, v := range startTypeCombo {
		if v == t {
			return i
		}
	}
	return -1
}

// startTypeByIndex returns the start type for a combo-box row; out-of-range
// falls back to auto.
func startTypeByIndex(i int) uint32 {
	if i < 0 || i >= len(startTypeCombo) {
		return windows.SERVICE_AUTO_START
	}
	return startTypeCombo[i]
}

// startTypeLabels returns the combo-box display strings in order.
func startTypeLabels() []string {
	out := make([]string, len(startTypeCombo))
	for i, v := range startTypeCombo {
		out[i] = startTypeLabel(v)
	}
	return out
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test -run TestStartType ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 5: 提交**

```powershell
git add server/starttype_windows.go server/starttype_windows_test.go
git commit -m "feat(service): add start-type arg/label/index mapping helpers" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 3: 设备快照 JSON 编解码（跨平台纯函数）

**Files:**
- Create: `server/status.go`
- Test: `server/status_test.go`

- [ ] **Step 1: 写失败测试**

Create `server/status_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func TestClientsCodecRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	in := []Client{
		{Name: "iPhone", Addr: "192.168.1.9", Since: now},
		{Name: "", Addr: "192.168.1.10", Since: now.Add(-time.Minute)},
	}
	b := encodeClients(in)
	out, err := decodeClients(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	for i := range in {
		if out[i].Name != in[i].Name || out[i].Addr != in[i].Addr || !out[i].Since.Equal(in[i].Since) {
			t.Errorf("row %d = %+v, want %+v", i, out[i], in[i])
		}
	}
}

func TestDecodeClientsEmpty(t *testing.T) {
	out, err := decodeClients(encodeClients(nil))
	if err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %d", len(out))
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test -run 'TestClientsCodec|TestDecodeClients' ./...`
Expected: FAIL（`undefined: encodeClients`）

- [ ] **Step 3: 实现编解码**

Create `server/status.go`:

```go
package main

import "encoding/json"

// encodeClients serializes a client snapshot for the status IPC pipe. Only the
// exported metadata (Name/Addr/Since) is emitted; the password is never part of
// this payload. Order is preserved so the UI shows the service's sort order.
func encodeClients(cs []Client) []byte {
	if cs == nil {
		cs = []Client{}
	}
	b, err := json.Marshal(cs)
	if err != nil {
		return []byte("[]")
	}
	return b
}

// decodeClients parses the payload produced by encodeClients.
func decodeClients(b []byte) ([]Client, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var out []Client
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
```

> 注：`Client.seq` 为非导出字段，不参与 JSON；解码后为 0。服务侧 `Snapshot()` 已排好序，编码保留顺序，UI 按收到顺序展示，无需再排序。

- [ ] **Step 4: 运行确认通过**

Run: `go test -run 'TestClientsCodec|TestDecodeClients' ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 5: 提交**

```powershell
git add server/status.go server/status_test.go
git commit -m "feat(status): add client-snapshot JSON codec for status IPC" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 4: ServingArbiter 决策 + 配置 diff 纯函数

**Files:**
- Create: `server/serving_windows.go`
- Test: `server/serving_windows_test.go`

> 本任务只建立纯函数与文件骨架；`servingController` 在 Task 6 加入同一文件。

- [ ] **Step 1: 写失败测试**

Create `server/serving_windows_test.go`:

```go
//go:build windows

package main

import "testing"

func TestWantStandalone(t *testing.T) {
	if wantStandalone(true) {
		t.Error("service running -> UI must NOT serve")
	}
	if !wantStandalone(false) {
		t.Error("service not running -> UI must serve")
	}
}

func TestNeedsRebind(t *testing.T) {
	a := serverConfig{Password: "p", Port: 27500, Name: "PC"}
	if needsRebind(a, a) {
		t.Error("identical config must not rebind")
	}
	b := a
	b.Port = 27600
	if !needsRebind(a, b) {
		t.Error("port change must rebind")
	}
	c := a
	c.Password = "x"
	if needsRebind(a, c) {
		t.Error("password-only change must not rebind")
	}
}

func TestNeedsReannounce(t *testing.T) {
	a := serverConfig{Password: "p", Port: 27500, Name: "PC"}
	if needsReannounce(a, a) {
		t.Error("identical config must not re-announce")
	}
	nameChg := a
	nameChg.Name = "TV"
	if !needsReannounce(a, nameChg) {
		t.Error("name change must re-announce")
	}
	portChg := a
	portChg.Port = 1
	if !needsReannounce(a, portChg) {
		t.Error("port change must re-announce")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test -run 'TestWantStandalone|TestNeedsRebind|TestNeedsReannounce' ./...`
Expected: FAIL（`undefined: wantStandalone`）

- [ ] **Step 3: 实现纯函数**

Create `server/serving_windows.go`:

```go
//go:build windows

package main

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
```

- [ ] **Step 4: 运行确认通过**

Run: `go test -run 'TestWantStandalone|TestNeedsRebind|TestNeedsReannounce' ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 5: 提交**

```powershell
git add server/serving_windows.go server/serving_windows_test.go
git commit -m "feat(serving): add arbiter decision and config-diff helpers" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 5: `Server.Serve(ln)` 拆分 + 加锁的密码/名称热更新

**Files:**
- Modify: `server/server.go`
- Test: `server/server_test.go`

- [ ] **Step 1: 写失败测试（密码热换）**

在 `server/server_test.go` 末尾追加。它用一个完整握手 helper，验证 `SetPassword` 后旧密码失败、新密码成功：

```go
// runHandshake performs hello→challenge→auth against s over an in-memory pipe
// and reports whether the server accepted (auth_ok). It closes the client when
// done.
func runHandshake(t *testing.T, s *Server, password string) bool {
	t.Helper()
	cli, srvConn := net.Pipe()
	go s.handle(srvConn)
	defer cli.Close()
	cli.SetDeadline(time.Now().Add(3 * time.Second))

	enc := json.NewEncoder(cli)
	sc := bufio.NewScanner(cli)

	if err := enc.Encode(map[string]any{"t": "hello", "name": "P"}); err != nil {
		t.Fatal(err)
	}
	if !sc.Scan() {
		t.Fatal("no challenge")
	}
	var ch struct {
		Salt  string `json:"salt"`
		Nonce string `json:"nonce"`
	}
	json.Unmarshal(sc.Bytes(), &ch)
	salt, _ := base64.StdEncoding.DecodeString(ch.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(ch.Nonce)
	if err := enc.Encode(map[string]any{"t": "auth", "proof": deriveProof(password, salt, nonce)}); err != nil {
		t.Fatal(err)
	}
	if !sc.Scan() {
		t.Fatal("no reply")
	}
	var reply struct {
		T string `json:"t"`
	}
	json.Unmarshal(sc.Bytes(), &reply)
	return reply.T == "auth_ok"
}

func TestSetPasswordHotSwap(t *testing.T) {
	s := &Server{password: "old", name: "srv", inj: newInjector(), reg: NewClientRegistry()}
	if !runHandshake(t, s, "old") {
		t.Fatal("old password should authenticate before swap")
	}
	s.SetPassword("new")
	if runHandshake(t, s, "old") {
		t.Error("old password must fail after swap")
	}
	if !runHandshake(t, s, "new") {
		t.Error("new password must authenticate after swap")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test -run TestSetPasswordHotSwap ./...`
Expected: FAIL（`s.SetPassword undefined`）

- [ ] **Step 3: 加锁字段 + getter/setter + `Serve` 拆分**

在 `server/server.go`：给 import 增加 `"sync"`（放进现有 import 块）。`Server` 结构体改为：

```go
type Server struct {
	mu       sync.RWMutex
	password string
	name     string
	inj      Injector
	reg      *ClientRegistry
}

// SetPassword hot-swaps the auth password; only new authentications are
// affected, already-authenticated connections stay up.
func (s *Server) SetPassword(p string) {
	s.mu.Lock()
	s.password = p
	s.mu.Unlock()
}

// SetName hot-swaps the display name returned in auth_ok.
func (s *Server) SetName(n string) {
	s.mu.Lock()
	s.name = n
	s.mu.Unlock()
}

func (s *Server) curPassword() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.password
}

func (s *Server) curName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.name
}
```

在 `handle` 中，把 `want := deriveProof(s.password, salt, nonce)` 改为 `want := deriveProof(s.curPassword(), salt, nonce)`；把 auth_ok 那行 `"server": s.name` 改为 `"server": s.curName()`。

把 `Listen` 拆成 `Serve(ln)` + 便捷 `Listen`：

```go
func (s *Server) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}

func (s *Server) Listen(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	log.Printf("control listening on tcp/%d", port)
	return s.Serve(ln)
}
```

> `&Server{password: ..., name: ...}` 字面量初始化保持可用（字段名未变），既有构造点无需改动。

- [ ] **Step 4: 运行确认通过（含 -race）**

Run: `go test -race -run 'TestSetPasswordHotSwap|TestHandleRegisters' ./...`
Expected: `ok  remotemouse/server`（无 DATA RACE）

- [ ] **Step 5: 提交**

```powershell
git add server/server.go server/server_test.go
git commit -m "refactor(server): split Serve(ln) and add locked password/name hot-swap" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 6: `servingController`（Start/Stop/Apply）

**Files:**
- Modify: `server/serving_windows.go`
- Test: `server/serving_windows_test.go`

- [ ] **Step 1: 写失败集成测试**

在 `server/serving_windows_test.go` 追加。用 `freePort` 取空闲端口，验证 Start 绑定、Apply 热换密码与改端口、Stop 释放：

```go
import 部分改为：

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)
```

追加：

```go
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// dialAuth runs a handshake against 127.0.0.1:port and reports auth_ok.
func dialAuth(t *testing.T, port int, password string) bool {
	t.Helper()
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	enc := json.NewEncoder(c)
	sc := bufio.NewScanner(c)
	enc.Encode(map[string]any{"t": "hello", "name": "P"})
	if !sc.Scan() {
		return false
	}
	var ch struct{ Salt, Nonce string }
	json.Unmarshal(sc.Bytes(), &ch)
	salt, _ := base64.StdEncoding.DecodeString(ch.Salt)
	nonce, _ := base64.StdEncoding.DecodeString(ch.Nonce)
	enc.Encode(map[string]any{"t": "auth", "proof": deriveProof(password, salt, nonce)})
	if !sc.Scan() {
		return false
	}
	var r struct{ T string }
	json.Unmarshal(sc.Bytes(), &r)
	return r.T == "auth_ok"
}

func TestServingControllerLifecycle(t *testing.T) {
	p1 := freePort(t)
	cfg := appConfig{pass: "old", name: "PC", port: p1}
	c := newServingController(cfg, NewClientRegistry())

	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !waitFor(func() bool { return dialAuth(t, p1, "old") }) {
		t.Fatal("service not accepting on p1 after Start")
	}

	// Hot password swap, same port.
	c.Apply(appConfig{pass: "new", name: "PC", port: p1})
	if dialAuth(t, p1, "old") {
		t.Error("old password must fail after Apply")
	}
	if !dialAuth(t, p1, "new") {
		t.Error("new password must work after Apply")
	}

	// Port change -> rebind.
	p2 := freePort(t)
	c.Apply(appConfig{pass: "new", name: "PC", port: p2})
	if !waitFor(func() bool { return dialAuth(t, p2, "new") }) {
		t.Error("must accept on p2 after port change")
	}

	c.Stop()
	if waitFor(func() bool { return dialAuth(t, p2, "new") }) {
		t.Error("must not accept after Stop")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test -run TestServingControllerLifecycle ./...`
Expected: FAIL（`undefined: newServingController`）

- [ ] **Step 3: 实现 controller**

在 `server/serving_windows.go` 顶部把包声明下的内容补上 import 与结构体（保留 Task 4 的三个纯函数）。完整文件应为：

```go
//go:build windows

package main

import (
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

// needsReannounce reports whether the mDNS advertisement must be refreshed.
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
```

在 import 块加入 `"fmt"`（`listenAddr` 需要）。最终 import 为：`"fmt"`, `"log"`, `"net"`, `"sync"`, `"github.com/libp2p/zeroconf/v2"`。

- [ ] **Step 4: 运行确认通过**

Run: `go test -run 'TestServingControllerLifecycle|TestWantStandalone|TestNeedsRebind|TestNeedsReannounce' ./...`
Expected: `ok  remotemouse/server`

> 若 mDNS 在无网络环境报错，`announceMDNS` 返回 nil 且不致命，测试仍应通过（只验证 TCP 行为）。

- [ ] **Step 5: 提交**

```powershell
git add server/serving_windows.go server/serving_windows_test.go
git commit -m "feat(serving): add servingController with Start/Stop/Apply hot-reconfig" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 7: 状态命名管道（服务侧 serve + UI 侧 query）+ 接入 `runServiceCore`

**Files:**
- Create: `server/status_windows.go`
- Modify: `server/service_windows.go`

> 管道 I/O 依赖 SYSTEM 服务与登录用户跨会话通信，无法在单测里可靠验证；本任务以 `go build` + 后续手工冒烟为验收（与既有 `installService` 无测试保持一致）。编解码已在 Task 3 覆盖。

- [ ] **Step 1: 实现管道两侧**

Create `server/status_windows.go`:

```go
//go:build windows

package main

import (
	"io"
	"log"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// statusPipe is the read-only IPC channel the running service exposes so the
// (non-admin) UI can display connected devices while the service owns the port.
// It carries only device metadata (name/addr/since); never the password.
const statusPipe = `\\.\pipe\remotemouse-status`

// serveStatusPipe answers each connection with a JSON snapshot of reg. It runs
// until stop is closed (which closes the listener and unblocks Accept).
func serveStatusPipe(name string, reg *ClientRegistry, stop <-chan struct{}) {
	ln, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: pipeSDDL})
	if err != nil {
		log.Printf("status pipe listen: %v", err)
		return
	}
	go func() {
		<-stop
		ln.Close()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			return // listener closed on stop
		}
		go func(conn net.Conn) {
			defer conn.Close()
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			conn.Write(encodeClients(reg.Snapshot()))
		}(c)
	}
}

// queryStatusPipe dials the service's status pipe and returns its device
// snapshot. Returns an error (handled as "empty") when the service is not
// exposing the pipe yet.
func queryStatusPipe() ([]Client, error) {
	to := 500 * time.Millisecond
	c, err := winio.DialPipe(statusPipe, &to)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	b, err := io.ReadAll(c)
	if err != nil {
		return nil, err
	}
	return decodeClients(b)
}
```

- [ ] **Step 2: 在 `runServiceCore` 起状态管道**

在 `server/service_windows.go` 的 `runServiceCore` 内，创建 `reg` 之后、启动 TCP 监听之前，加一行 goroutine（`reg` 变量即 `NewClientRegistry()` 的返回）：

找到：

```go
	pi := newPipeInjector(nil)
	reg := NewClientRegistry()
	srv := &Server{password: cfg.pass, name: cfg.name, inj: pi, reg: reg}
```

在其后加入：

```go
	go serveStatusPipe(statusPipe, reg, stop)
```

- [ ] **Step 3: 编译**

Run: `go build ./...`
Expected: 无输出（成功）

- [ ] **Step 4: 运行既有测试确保未回归**

Run: `go test ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 5: 提交**

```powershell
git add server/status_windows.go server/service_windows.go
git commit -m "feat(status): expose service device snapshot over a status pipe" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 8: 服务操作 + 只读启动类型查询

**Files:**
- Modify: `server/service_windows.go`
- Modify: `server/ui_service_windows.go`

> SCM 变更类操作需管理员、有副作用，无单测（与 `installService` 一致），以 `go build` 验收。只读 `serviceStartType()` 亦以编译 + 后续手工冒烟验收。

- [ ] **Step 1: 加服务操作函数**

在 `server/service_windows.go` 末尾追加（文件已 import `svc`、`mgr`、`registry`、`fmt`、`log`、`time`）：

```go
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

// stopService stops the service and waits briefly for it to leave RUNNING (admin).
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
	fmt.Printf("stopped service %q\n", svcName)
	return nil
}

// restartService stops then starts the service (admin). A stop error when the
// service is already stopped is tolerated.
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
```

- [ ] **Step 2: 加只读启动类型查询**

在 `server/ui_service_windows.go` 末尾追加（文件已 import `windows`、`registry`；需要额外 import `"golang.org/x/sys/windows/svc/mgr"`——加入 import 块）：

```go
// serviceStartType returns the service start type using read-only SCM access
// (no admin). ok is false when the service is not installed or unreadable. It
// wraps a self-opened handle in mgr.Service to reuse mgr's config parsing while
// avoiding mgr.Connect (which demands admin).
func serviceStartType() (uint32, bool) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, false
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString(svcName)
	h, err := windows.OpenService(scm, name, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return 0, false // not installed
	}
	defer windows.CloseServiceHandle(h)
	s := &mgr.Service{Name: svcName, Handle: h}
	cfg, err := s.Config()
	if err != nil {
		return 0, false
	}
	return cfg.StartType, true
}

// serviceInstalled reports whether the service exists (read-only).
func serviceInstalled() bool {
	_, ok := serviceStartType()
	return ok
}
```

- [ ] **Step 3: 硬化 `elevatedSelf` 参数拼接（先写失败测试）**

现有 `elevatedSelf` 用 `strings.Join(args, " ")` 拼命令行——一旦密码或设备名含空格（如 `-name "Living Room"`），子进程 `flag` 解析会错位。抽出纯函数 `elevatedArgLine` 用 `syscall.EscapeArg` 逐个转义，便于单测。

新建 `server/ui_service_windows_test.go`：

```go
//go:build windows

package main

import "testing"

func TestElevatedArgLine(t *testing.T) {
	got := elevatedArgLine([]string{"-apply-config", "-name", "Living Room", "-pass", "p q", "-port", "27500"})
	want := `-apply-config -name "Living Room" -pass "p q" -port 27500`
	if got != want {
		t.Fatalf("elevatedArgLine = %q, want %q", got, want)
	}
}
```

- [ ] **Step 4: 运行确认失败**

Run: `go test -run TestElevatedArgLine ./...`
Expected: FAIL（`undefined: elevatedArgLine`）

- [ ] **Step 5: 实现 `elevatedArgLine` 并接入 `elevatedSelf`**

在 `server/ui_service_windows.go` 的 import 块加入 `"syscall"`（`strings` 已在）。在 `elevatedSelf` 上方新增纯函数：

```go
// elevatedArgLine builds a Windows command line from args, quoting/escaping each
// one so values containing spaces (e.g. a device name or password) survive the
// child process's flag parsing. syscall.EscapeArg applies the CommandLineToArgvW
// quoting rules and leaves space-free tokens untouched.
func elevatedArgLine(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = syscall.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}
```

把 `elevatedSelf` 内的这一行：

```go
	argp, _ := windows.UTF16PtrFromString(strings.Join(args, " "))
```

改为：

```go
	argp, _ := windows.UTF16PtrFromString(elevatedArgLine(args))
```

- [ ] **Step 6: 运行确认通过**

Run: `go test -run TestElevatedArgLine ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 7: 编译**

Run: `go build ./...`
Expected: 无输出（成功）

- [ ] **Step 8: 提交**

```powershell
git add server/service_windows.go server/ui_service_windows.go server/ui_service_windows_test.go
git commit -m "feat(service): add start/stop/restart/set-start-type ops and read-only start-type query" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 9: 新 CLI flag + `mode()` + `run()` 派发

**Files:**
- Modify: `server/main.go`
- Modify: `server/main_test.go`
- Modify: `server/run_windows.go`

- [ ] **Step 1: 更新 mode 测试**

在 `server/main_test.go` 的 `cases` 切片里追加用例（放在 `standalone` 之后、`install wins` 之前）：

```go
		{"start-service", appConfig{startService: true}, "start-service"},
		{"stop-service", appConfig{stopService: true}, "stop-service"},
		{"set-start-type", appConfig{setStartType: "auto"}, "set-start-type"},
		{"apply-config", appConfig{applyConfig: true}, "apply-config"},
		{"install wins over start", appConfig{installService: true, startService: true}, "install-service"},
```

- [ ] **Step 2: 运行确认失败**

Run: `go test -run TestAppConfigMode ./...`
Expected: FAIL（`unknown field 'startService'`）

- [ ] **Step 3: 加字段、flag、mode 分支**

在 `server/main.go` 的 `appConfig` 结构体里，`injectLog` 之后追加：

```go
	startService   bool
	stopService    bool
	setStartType   string
	applyConfig    bool
```

在 `main()` 的 flag 定义区（`injectLog` 那行之后）追加：

```go
	flag.BoolVar(&cfg.startService, "start-service", false, "start the Windows service (run as admin)")
	flag.BoolVar(&cfg.stopService, "stop-service", false, "stop the Windows service (run as admin)")
	flag.StringVar(&cfg.setStartType, "set-start-type", "", "set service start type: auto|manual|disabled (run as admin)")
	flag.BoolVar(&cfg.applyConfig, "apply-config", false, "write HKLM config and restart the service if running (run as admin)")
```

把 `mode()` 改为（在 install/uninstall 之后、`service` 之前插入新分支，保持 install/uninstall 最高优先级）：

```go
func (c appConfig) mode() string {
	switch {
	case c.installService:
		return "install-service"
	case c.uninstallService:
		return "uninstall-service"
	case c.startService:
		return "start-service"
	case c.stopService:
		return "stop-service"
	case c.setStartType != "":
		return "set-start-type"
	case c.applyConfig:
		return "apply-config"
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
```

- [ ] **Step 4: 运行确认通过**

Run: `go test -run TestAppConfigMode ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 5: 在 `run()` 派发新 mode**

在 `server/run_windows.go` 的 switch 中，`uninstall-service` case 之后加入：

```go
	case "start-service":
		if err := startService(); err != nil {
			log.Fatalf("start-service: %v", err)
		}
	case "stop-service":
		if err := stopService(); err != nil {
			log.Fatalf("stop-service: %v", err)
		}
	case "set-start-type":
		t, ok := startTypeFromArg(cfg.setStartType)
		if !ok {
			log.Fatalf("set-start-type: invalid value %q (want auto|manual|disabled)", cfg.setStartType)
		}
		if err := setStartType(t); err != nil {
			log.Fatalf("set-start-type: %v", err)
		}
	case "apply-config":
		if err := applyConfigElevated(cfg); err != nil {
			log.Fatalf("apply-config: %v", err)
		}
```

- [ ] **Step 6: 编译 + 全量测试**

Run: `go build ./...; go test ./...`
Expected: build 无输出；测试 `ok  remotemouse/server`

- [ ] **Step 7: 提交**

```powershell
git add server/main.go server/main_test.go server/run_windows.go
git commit -m "feat(cli): add start/stop/set-start-type/apply-config modes" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 10: 控制面板窗口（`serveUI` 重写 + `runControlPanel`）

**Files:**
- Modify: `server/ui_service_windows.go`（重写 `serveUI`）
- Create: `server/ui_panel_windows.go`

> GUI 无单测，以 `go build` + 手工冒烟验收。`runUI`（旧简易窗口）保留给 `-standalone` 用，不改。共享 helper（`deviceModel`、`loadIcon`、`showWindow`、`copyClip`、`fmtDur`、`trayICO`）继续来自 `ui_windows.go`。

- [ ] **Step 1: 重写 `serveUI`**

把 `server/ui_service_windows.go` 顶部的 `serveUI` 整个替换为：

```go
// serveUI is the default (no-flag) mode: it opens the control panel. Serving
// (owning the TCP port) is arbitrated inside the panel by a servingController
// that follows the service state, so we never double-bind the port.
func serveUI(cfg appConfig) {
	// Prefer persisted HKLM config when the service is installed, so the panel
	// shows the values the service actually uses.
	if serviceInstalled() {
		if sc, err := readConfig(registry.LOCAL_MACHINE); err == nil {
			cfg.pass, cfg.port = sc.Password, sc.Port
			if sc.Name != "" {
				cfg.name = sc.Name
			}
		}
	}
	reg := NewClientRegistry()
	ctrl := newServingController(cfg, reg)
	ips := DetectIPs()
	runControlPanel(cfg, ctrl, reg, ips)
}
```

保留文件其余部分（`elevatedSelf`、`serviceRunning`、`serviceState`，以及 Task 8 加的 `serviceStartType`/`serviceInstalled`）。确保 import 含 `registry`（已在）。

- [ ] **Step 2: 新建控制面板窗口**

Create `server/ui_panel_windows.go`:

```go
//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// runControlPanel shows the integrated control panel: editable connection
// settings, live service management, and the connected-devices table. It owns
// a StatusTicker goroutine that (1) reads service state + start type, (2) drives
// the ServingArbiter to acquire/release the port, and (3) refreshes the device
// table from the local registry (standalone) or the status pipe (service mode).
func runControlPanel(cfg appConfig, ctrl *servingController, reg *ClientRegistry, ips HostIPs) {
	if cfg.notray {
		// Headless: still run the arbiter loop so serving follows the service.
		runArbiterHeadless(ctrl)
		return
	}
	runtime.LockOSThread()

	primary := ips.Primary
	if primary == "" {
		primary = "未检测到局域网 IP"
	}
	secText := strings.Join(ips.Secondary, "   ")

	var mw *walk.MainWindow
	var portEdit, passEdit, nameEdit *walk.LineEdit
	var secLabel, devCount, statusLabel *walk.Label
	var startCombo *walk.ComboBox
	var btnInstall, btnStart, btnStop, btnUninstall, btnApply, btnShowPass *walk.PushButton
	model := &deviceModel{reg: reg}

	// applyGuard suppresses the combo's OnCurrentIndexChanged while the ticker
	// programmatically sets its index.
	applyGuard := false

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "RemoteMouse 控制面板",
		MinSize:  Size{Width: 460, Height: 520},
		Size:     Size{Width: 480, Height: 560},
		Layout:   VBox{},
		Children: []Widget{
			GroupBox{
				Title:  "连接信息",
				Layout: Grid{Columns: 3},
				Children: []Widget{
					Label{Text: "推荐 IP"},
					Label{Text: primary, Font: Font{Bold: true, PointSize: 11}},
					PushButton{Text: "复制", OnClicked: func() { copyClip(ips.Primary) }},

					Label{Text: "端口"},
					LineEdit{AssignTo: &portEdit, Text: strconv.Itoa(cfg.port)},
					PushButton{Text: "复制", OnClicked: func() { copyClip(portEdit.Text()) }},

					Label{Text: "密码"},
					LineEdit{AssignTo: &passEdit, Text: cfg.pass, PasswordMode: true},
					PushButton{AssignTo: &btnShowPass, Text: "显示", OnClicked: func() {
						show := passEdit.PasswordMode()
						passEdit.SetPasswordMode(!show)
						if show {
							btnShowPass.SetText("隐藏")
						} else {
							btnShowPass.SetText("显示")
						}
					}},

					Label{Text: "设备名"},
					LineEdit{AssignTo: &nameEdit, Text: cfg.name},
					PushButton{Text: "复制", OnClicked: func() { copyClip(nameEdit.Text()) }},

					Label{Text: "其他地址"},
					Label{AssignTo: &secLabel, Text: secText, Visible: false},
					PushButton{Text: "展开", Enabled: secText != "", OnClicked: func() {
						secLabel.SetVisible(!secLabel.Visible())
					}},

					Label{Text: ""},
					PushButton{AssignTo: &btnApply, Text: "保存并应用"},
					Label{Text: "改后点这里生效"},
				},
			},
			GroupBox{
				Title:  "服务管理",
				Layout: Grid{Columns: 4},
				Children: []Widget{
					Label{Text: "状态"},
					Label{AssignTo: &statusLabel, Text: "查询中…", Font: Font{Bold: true}, ColumnSpan: 3},

					Label{Text: "启动类型"},
					ComboBox{AssignTo: &startCombo, Model: startTypeLabels(), ColumnSpan: 3},

					PushButton{AssignTo: &btnInstall, Text: "安装并启动"},
					PushButton{AssignTo: &btnStart, Text: "启动"},
					PushButton{AssignTo: &btnStop, Text: "停止"},
					PushButton{AssignTo: &btnUninstall, Text: "卸载"},

					Label{Text: "管理操作需要管理员权限（会弹 UAC）", ColumnSpan: 4},
				},
			},
			Label{AssignTo: &devCount, Text: "已连接设备 (0)"},
			TableView{
				MinSize: Size{Height: 150},
				Columns: []TableViewColumn{
					{Title: "设备名", Width: 130},
					{Title: "IP", Width: 130},
					{Title: "时长", Width: 80},
					{Title: "状态", Width: 60},
				},
				Model: model,
			},
		},
	}).Create(); err != nil {
		log.Printf("control panel init failed, running headless: %v", err)
		runArbiterHeadless(ctrl)
		return
	}

	// --- Save & Apply ---
	btnApply.Clicked().Attach(func() {
		port, err := strconv.Atoi(strings.TrimSpace(portEdit.Text()))
		if err != nil || port < 1 || port > 65535 {
			walk.MsgBox(mw, "端口无效", "端口需为 1–65535 的整数。", walk.MsgBoxIconWarning)
			return
		}
		pass := passEdit.Text()
		if pass == "" {
			walk.MsgBox(mw, "密码为空", "密码不能为空。", walk.MsgBoxIconWarning)
			return
		}
		next := appConfig{pass: pass, port: port, name: strings.TrimSpace(nameEdit.Text())}
		if serviceInstalled() {
			// Persist to HKLM (+ restart if running) via one elevated call.
			elevatedSelf("-apply-config", "-pass", pass, "-port", strconv.Itoa(port), "-name", next.name)
			// If we are currently the one serving (installed but stopped), also
			// hot-apply so the live standalone server reflects the change.
			ctrl.Apply(next)
		} else {
			ctrl.Apply(next)
			if autostartOn() {
				setAutostart(true, pass, port) // rewrite Run command with new values
			}
		}
	})

	// --- Service buttons (elevated) ---
	btnInstall.Clicked().Attach(func() {
		elevatedSelf("-install-service", "-pass", passEdit.Text(), "-port", portEdit.Text())
	})
	btnStart.Clicked().Attach(func() { elevatedSelf("-start-service") })
	btnStop.Clicked().Attach(func() { elevatedSelf("-stop-service") })
	btnUninstall.Clicked().Attach(func() { elevatedSelf("-uninstall-service") })

	// --- Start-type combo (elevated) ---
	startCombo.CurrentIndexChanged().Attach(func() {
		if applyGuard {
			return
		}
		t := startTypeByIndex(startCombo.CurrentIndex())
		elevatedSelf("-set-start-type", startTypeArg(t))
	})

	// Tray icon + minimal menu.
	if ni, err := walk.NewNotifyIcon(mw); err == nil {
		defer ni.Dispose()
		if ic := loadIcon(); ic != nil {
			ni.SetIcon(ic)
			mw.SetIcon(ic)
		}
		ni.SetToolTip("RemoteMouse  " + primary)
		ni.SetVisible(true)
		ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
			if button != walk.LeftButton {
				return
			}
			if mw.Visible() {
				mw.SetVisible(false)
			} else {
				showWindow(mw)
			}
		})
		showAct := walk.NewAction()
		showAct.SetText("显示窗口")
		showAct.Triggered().Attach(func() { showWindow(mw) })
		ni.ContextMenu().Actions().Add(showAct)

		loginAct := walk.NewAction()
		loginAct.SetText("登录时打开界面")
		loginAct.SetChecked(autostartOn())
		loginAct.Triggered().Attach(func() {
			on := !loginAct.Checked()
			port, _ := strconv.Atoi(portEdit.Text())
			setAutostart(on, passEdit.Text(), port)
			loginAct.SetChecked(on)
		})
		ni.ContextMenu().Actions().Add(loginAct)

		ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())
		quitAct := walk.NewAction()
		quitAct.SetText("退出")
		quitAct.Triggered().Attach(func() { walk.App().Exit(0) })
		ni.ContextMenu().Actions().Add(quitAct)
	} else {
		log.Printf("tray init failed: %v", err)
	}

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		*canceled = true
		mw.SetVisible(false)
	})

	// Live device refresh for standalone mode (push). Service mode is polled by
	// the ticker below.
	reg.SetOnChange(func() {
		mw.Synchronize(func() {
			if ctrl.isRunning() {
				model.items = reg.Snapshot()
				model.PublishRowsReset()
				devCount.SetText(fmt.Sprintf("已连接设备 (%d)", len(model.items)))
			}
		})
	})

	// StatusTicker: arbitrate serving + refresh status/devices every second.
	// All blocking I/O (SCM, pipe) happens here on a background goroutine; UI
	// mutations go through mw.Synchronize.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		tick := func() {
			running := serviceRunning()
			// Arbitrate: own the port iff the service is not running.
			if wantStandalone(running) {
				ctrl.Start()
			} else {
				ctrl.Stop()
			}
			state := serviceState()
			stype, installed := serviceStartType()
			var devices []Client
			if ctrl.isRunning() {
				devices = reg.Snapshot()
			} else if list, err := queryStatusPipe(); err == nil {
				devices = list
			}
			mw.Synchronize(func() {
				applyPanelState(panelWidgets{
					statusLabel: statusLabel, startCombo: startCombo,
					btnInstall: btnInstall, btnStart: btnStart, btnStop: btnStop,
					btnUninstall: btnUninstall, devCount: devCount, model: model,
				}, state, stype, installed, devices, &applyGuard)
			})
		}
		tick() // immediate first paint
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				tick()
			}
		}
	}()

	mw.Run()
}

// panelWidgets bundles the widgets the ticker updates.
type panelWidgets struct {
	statusLabel  *walk.Label
	startCombo   *walk.ComboBox
	btnInstall   *walk.PushButton
	btnStart     *walk.PushButton
	btnStop      *walk.PushButton
	btnUninstall *walk.PushButton
	devCount     *walk.Label
	model        *deviceModel
}

// applyPanelState updates all live widgets from a status snapshot. Runs on the
// GUI thread. guard is toggled around the combo write so the change handler
// does not treat a programmatic set as a user action.
func applyPanelState(w panelWidgets, state string, stype uint32, installed bool, devices []Client, guard *bool) {
	if installed {
		w.statusLabel.SetText(fmt.Sprintf("● %s（%s）", state, startTypeLabel(stype)))
	} else {
		w.statusLabel.SetText("● 未安装")
	}

	*guard = true
	if idx := startTypeIndex(stype); installed && idx >= 0 {
		w.startCombo.SetCurrentIndex(idx)
		w.startCombo.SetEnabled(true)
	} else {
		w.startCombo.SetEnabled(false)
	}
	*guard = false

	// Button matrix.
	w.btnInstall.SetEnabled(!installed)
	w.btnStart.SetEnabled(installed && state == "已停止")
	w.btnStop.SetEnabled(installed && state == "运行中")
	w.btnUninstall.SetEnabled(installed)

	w.model.items = devices
	w.model.PublishRowsReset()
	w.devCount.SetText(fmt.Sprintf("已连接设备 (%d)", len(devices)))
}

// runArbiterHeadless keeps serving following the service state without a window
// (notray). Blocks forever.
func runArbiterHeadless(ctrl *servingController) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		if wantStandalone(serviceRunning()) {
			ctrl.Start()
		} else {
			ctrl.Stop()
		}
	}
}
```

- [ ] **Step 3: 编译**

Run: `go build ./...`
Expected: 无输出（成功）。

- [ ] **Step 4: 全量测试未回归**

Run: `go test ./...`
Expected: `ok  remotemouse/server`

- [ ] **Step 5: 生成带资源的可执行并手工冒烟**

Run:
```powershell
Remove-Item rsrc.syso -ErrorAction SilentlyContinue
.\build.ps1
.\rmserver.exe -pass 1234 -port 27500
```
手工验收（无需真机）：
- 窗口标题为「RemoteMouse 控制面板」，连接信息中端口/密码/设备名为可编辑输入框，密码框「显示/隐藏」可切换。
- 「服务管理」显示「● 未安装」，仅「安装并启动」可点，启动类型下拉禁用。
- 点「安装并启动」→ 弹 UAC；同意后约 1–2 秒状态变「● 运行中（自动）」，「停止/卸载」可点、启动类型下拉可用。
- 改端口→「保存并应用」→ 弹一次 UAC；状态短暂变「停止中/启动中」后回「运行中」。
- 点「卸载」→ UAC → 状态回「● 未安装」，且窗口重新开始本地 serving（端口被本进程接管，不报错）。
- 关窗到托盘；托盘菜单为「显示窗口 / 登录时打开界面 / 退出」。

- [ ] **Step 6: 提交**

```powershell
git add server/ui_service_windows.go server/ui_panel_windows.go
git commit -m "feat(ui): integrated control panel with live status, editable config, and port arbitration" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 11: 登录自启改名 + Run 命令带 `-name`

**Files:**
- Modify: `server/autostart_windows.go`
- Modify: `server/ui_windows.go`（旧简易窗口托盘项改名，保持一致）

- [ ] **Step 1: Run 命令带设备名**

在 `server/autostart_windows.go` 把 `autostartCmd` 改为读取当前 HKCU 已存端口/密码之外，直接接受 name。最简做法：保留签名 `setAutostart(on, pass, port)`，命令里追加从 HKLM/flags 无关的 name 较复杂——改为让命令包含调用方传入的 name。为最小改动，扩展 `autostartCmd` 增加 name 参数并给 `setAutostart` 增加可选 name：

```go
func autostartCmd(pass string, port int, name string) string {
	exe, _ := os.Executable()
	cmd := `"` + exe + `" -pass ` + pass + " -port " + strconv.Itoa(port)
	if strings.TrimSpace(name) != "" {
		cmd += ` -name "` + name + `"`
	}
	return cmd
}
```

在文件顶部 import 加入 `"strings"`。把 `setAutostart` 改为：

```go
func setAutostart(on bool, pass string, port int) {
	setAutostartNamed(on, pass, port, "")
}

func setAutostartNamed(on bool, pass string, port int, name string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if on {
		k.SetStringValue(runVal, autostartCmd(pass, port, name))
	} else {
		k.DeleteValue(runVal)
	}
}
```

> 保留 `setAutostart(on, pass, port)` 旧签名（内部转调），使 `ui_windows.go`、`ui_panel_windows.go` 现有调用不破。控制面板若要写入 name，可改调 `setAutostartNamed`。

- [ ] **Step 2: 控制面板写入 name（可选增强）**

在 `server/ui_panel_windows.go` 中，把 Save&Apply 的 `setAutostart(true, pass, port)` 与托盘 `loginAct` 的 `setAutostart(on, passEdit.Text(), port)` 分别改为 `setAutostartNamed(true, pass, port, next.name)` 和 `setAutostartNamed(on, passEdit.Text(), port, nameEdit.Text())`。

- [ ] **Step 3: 旧窗口托盘项改名**

在 `server/ui_windows.go` 里把托盘 `autoAct.SetText("开机自启")` 改为 `autoAct.SetText("登录时打开界面")`，与控制面板措辞一致（避免与服务"开机自启"混淆）。其余不动。

- [ ] **Step 4: 编译 + 测试**

Run: `go build ./...; go test ./...`
Expected: build 无输出；测试 `ok`

- [ ] **Step 5: 提交**

```powershell
git add server/autostart_windows.go server/ui_panel_windows.go server/ui_windows.go
git commit -m "feat(autostart): include -name in Run command; rename login-autostart label" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 12: 文档更新

**Files:**
- Modify: `server/README.md`

- [ ] **Step 1: 更新运行/安装章节**

在 `server/README.md` 的「运行 Windows」与「安装（开机自启）」之间，加入控制面板说明段落：

```markdown
## 控制面板（推荐）

双击或 `.\rmserver.exe -pass 1234 -port 27500` 打开「RemoteMouse 控制面板」单窗口即可完成一切：

- **连接信息**：推荐 IP（只读+复制）、端口/密码/设备名**可编辑**，「保存并应用」自动生效（服务在运行则重启服务，约 1 秒；独立运行则进程内热重配，已连接手机不掉线）。
- **服务管理**：实时状态；「启动类型」下拉（自动/手动/禁用，即"服务开机自启"）；「安装并启动 / 启动 / 停止 / 卸载」按钮按状态启停用。管理操作会弹 UAC 请求管理员。
- **已连接设备**：独立模式实时显示；服务模式经状态管道显示（约 1 秒刷新）。

命令行等价物（供脚本/排障，均需管理员）：

    .\rmserver.exe -install-service -pass 1234 -port 27500
    .\rmserver.exe -uninstall-service
    .\rmserver.exe -start-service
    .\rmserver.exe -stop-service
    .\rmserver.exe -set-start-type auto|manual|disabled
    .\rmserver.exe -apply-config -pass 1234 -port 27500 -name 客厅PC
```

把原「安装（开机自启）」小节中，托盘「开机自启」的措辞更新为「登录时打开界面」，并说明它与服务「启动类型=自动」的区别（前者是登录后打开本界面，后者是服务随机器启动）。

- [ ] **Step 2: 提交**

```powershell
git add server/README.md
git commit -m "docs(server): document the control panel and new service CLI modes" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 13: 最终验证与自检

- [ ] **Step 1: 全量构建与测试（含 -race）**

Run:
```powershell
go vet ./...
go build ./...
go test ./...
go test -race -run 'TestSetPasswordHotSwap|TestServingControllerLifecycle' ./...
```
Expected: `go vet` 无输出；build 无输出；两次 test 均 `ok  remotemouse/server`。

- [ ] **Step 2: 出带资源的 exe**

Run:
```powershell
Remove-Item rsrc.syso -ErrorAction SilentlyContinue
.\build.ps1
```
Expected: `built rmserver.exe`

- [ ] **Step 3: 端到端手工冒烟（对照 Task 10 Step 5 清单全过一遍）**

覆盖：未安装→安装→改端口保存→设启动类型为手动→停止→启动→卸载；每步观察状态标签、按钮启停用、UAC 行为符合预期；若有 iOS 真机，验证连接与"保存并应用"改密码后重连生效，服务模式下设备出现在列表中。

- [ ] **Step 4: 规范自检（对照 spec 勾项）**

逐条核对 `docs/superpowers/specs/2026-07-01-control-panel-design.md` 的 §3 决策表 D1–D7、§5 布局、§7 交互矩阵、§8 CLI、§12 测试计划，确认均有对应实现与提交。

---

## Self-Review（计划作者已核对）

- **规范覆盖**：D1 面板整合→T10；D2 可编辑字段→T10；D3 保存自动应用→T6/T8/T10；D4 每操作提权→T8/T9/T10；D5 端口仲裁→T4/T6/T10；D6 服务模式设备列表→T3/T7/T10；D7 启动类型=开机自启 + 自启改名→T2/T8/T10/T11。§12 测试计划：config(T1)/starttype(T2)/codec(T3)/arbiter+diff(T4)/SetPassword(T5)/controller(T6)/mode(T9) 均落测。
- **占位符**：无 TODO/TBD；GUI 任务以具体代码 + 明确手工验收清单替代自动化测试（GUI 一贯不测，符合既有实践）。
- **类型一致**：`serverConfig{Password,Port,Name}`、`servingController.{Start,Stop,Apply,isRunning}`、`serviceStartType()(uint32,bool)`、`startType{FromArg,Arg,Label,Index,ByIndex,Labels}`、`encode/decodeClients`、`Server.{Serve,SetPassword,SetName,curPassword,curName}` 在各任务间签名一致。
- **已知取舍**（写入 spec §13）：状态管道 1 秒轮询延迟；HKLM 密码明文（既有 TODO）；SDDL 宽松（POC）。
