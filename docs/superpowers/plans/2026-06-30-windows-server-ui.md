# Windows Server 可视化 UI 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Windows server 从「仅托盘菜单」升级为带主窗口的可视化界面，展示推荐 IP/端口/密码与实时在线设备列表，托盘作为显隐开关入口。

**Architecture:** 新增跨平台 `ClientRegistry`（连接状态中枢）与 `netinfo`（本机 IP 探测），让控制面 `server.go` 与 UI 解耦；Windows 用 `lxn/walk`（原生 Win32，cgo-free）实现主窗口 + `NotifyIcon` 托盘，替换 `getlantern/systray`；mac/other 无窗口、行为不变。

**Tech Stack:** Go 1.26、`github.com/lxn/walk` + `github.com/lxn/win`、`github.com/akavel/rsrc`（嵌入 manifest+图标）。

---

## 文件结构

| 文件 | 责任 |
|------|------|
| `server/clients.go` 🆕 | `ClientRegistry`：线程安全在线连接表 + 变更订阅（跨平台） |
| `server/clients_test.go` 🆕 | `ClientRegistry` 单测（含并发） |
| `server/netinfo.go` 🆕 | 枚举网卡，推荐局域网 IP + 次要 IP（跨平台） |
| `server/netinfo_test.go` 🆕 | `pickIPs` 纯函数单测 |
| `server/server.go` ✏️ | `Server` 增 `reg` 字段；`handle` 认证后 `Register`/`defer Deregister`；`ipOf` helper |
| `server/server_test.go` 🆕 | `handle` 走完认证后注册/断开注销的集成测试（net.Pipe） |
| `server/main.go` ✏️ | 创建 `ClientRegistry`、`DetectIPs()`，串入 `Server` 与 `runUI` |
| `server/ui_windows.go` 🆕 | walk 主窗口 + 托盘（替换 `tray_windows.go` 的 UI 部分） |
| `server/autostart_windows.go` 🆕 | 注册表开机自启（从 `tray_windows.go` 拆出） |
| `server/ui_other.go` ♻️ | 原 `tray_other.go` 重命名，`runUI` 签名扩展 |
| `server/assets/manifest.xml` 🆕 | Common Controls v6 + DPI 感知 |
| `server/build.ps1` ✏️ | 缺 `rsrc.syso` 则用 `rsrc` 生成后再 `go build` |

依赖：移除 `github.com/getlantern/systray`；新增 `github.com/lxn/walk`、`github.com/lxn/win`。

每个 Task 结束时代码可编译、可提交。Task 1–3 走严格 TDD；Task 5（GUI）无法单测，靠 `go build` 编译验证 + 手动验收。

---

### Task 1: ClientRegistry（连接状态中枢）

**Files:**
- Test: `server/clients_test.go`
- Create: `server/clients.go`

- [ ] **Step 1: 写失败测试**

Create `server/clients_test.go`:

```go
package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRegistryRegisterSnapshot(t *testing.T) {
	r := NewClientRegistry()
	id1 := r.Register("iPhone", "10.0.0.2")
	r.Register("iPad", "10.0.0.3")
	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("want 2 clients, got %d", len(snap))
	}
	if snap[0].Name != "iPhone" { // sorted by Since ascending
		t.Errorf("want iPhone first, got %q", snap[0].Name)
	}
	r.Deregister(id1)
	if len(r.Snapshot()) != 1 {
		t.Errorf("want 1 after deregister, got %d", len(r.Snapshot()))
	}
}

func TestRegistryOnChange(t *testing.T) {
	r := NewClientRegistry()
	var n int32
	r.SetOnChange(func() { atomic.AddInt32(&n, 1) })
	id := r.Register("a", "1.1.1.1") // +1
	r.Deregister(id)                 // +1
	r.Deregister(id)                 // no-op, must not fire
	if got := atomic.LoadInt32(&n); got != 2 {
		t.Errorf("want 2 onChange calls, got %d", got)
	}
}

func TestRegistrySnapshotIsolated(t *testing.T) {
	r := NewClientRegistry()
	r.Register("a", "1.1.1.1")
	snap := r.Snapshot()
	snap[0].Name = "mutated"
	if r.Snapshot()[0].Name != "a" {
		t.Error("Snapshot must return an isolated copy")
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewClientRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := r.Register("x", "1.1.1.1")
			_ = r.Snapshot()
			r.Deregister(id)
		}()
	}
	wg.Wait()
	if len(r.Snapshot()) != 0 {
		t.Errorf("want 0 after all deregister, got %d", len(r.Snapshot()))
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test -run TestRegistry ./...`
Expected: FAIL（`undefined: NewClientRegistry`）

- [ ] **Step 3: 实现**

Create `server/clients.go`:

```go
package main

import (
	"sort"
	"sync"
	"time"
)

// Client is one authenticated, currently-connected device.
type Client struct {
	Name  string
	Addr  string
	Since time.Time
}

// ClientRegistry tracks online connections and notifies a subscriber on change.
// Safe for concurrent use.
type ClientRegistry struct {
	mu       sync.RWMutex
	clients  map[uint64]Client
	nextID   uint64
	onChange func()
}

func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{clients: make(map[uint64]Client)}
}

// Register adds a connection and returns its id. Triggers onChange.
func (r *ClientRegistry) Register(name, addr string) uint64 {
	r.mu.Lock()
	r.nextID++
	id := r.nextID
	r.clients[id] = Client{Name: name, Addr: addr, Since: time.Now()}
	r.mu.Unlock()
	r.notify()
	return id
}

// Deregister removes a connection by id. Triggers onChange only if it existed.
func (r *ClientRegistry) Deregister(id uint64) {
	r.mu.Lock()
	_, ok := r.clients[id]
	delete(r.clients, id)
	r.mu.Unlock()
	if ok {
		r.notify()
	}
}

// Snapshot returns an isolated copy of current clients, sorted by Since ascending.
func (r *ClientRegistry) Snapshot() []Client {
	r.mu.RLock()
	out := make([]Client, 0, len(r.clients))
	for _, c := range r.clients {
		out = append(out, c)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

// SetOnChange registers the change callback (UI). Called once at setup.
func (r *ClientRegistry) SetOnChange(f func()) {
	r.mu.Lock()
	r.onChange = f
	r.mu.Unlock()
}

func (r *ClientRegistry) notify() {
	r.mu.RLock()
	f := r.onChange
	r.mu.RUnlock()
	if f != nil {
		f()
	}
}
```

- [ ] **Step 4: 运行测试（含竞态检测），确认通过**

Run: `cd server; go test -race -run TestRegistry ./...`
Expected: PASS（4 个测试通过，无竞态报告）

- [ ] **Step 5: 提交**

```powershell
cd server; gofmt -w clients.go clients_test.go
git add server/clients.go server/clients_test.go
git commit -m "feat(server): add concurrent ClientRegistry for live connections" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: 本机 IP 探测（netinfo）

**Files:**
- Test: `server/netinfo_test.go`
- Create: `server/netinfo.go`

- [ ] **Step 1: 写失败测试**

Create `server/netinfo_test.go`:

```go
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
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test -run TestPickIPs ./...`
Expected: FAIL（`undefined: pickIPs` / `ifaceAddrs`）

- [ ] **Step 3: 实现**

Create `server/netinfo.go`:

```go
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
```

- [ ] **Step 4: 运行测试，确认通过**

Run: `cd server; go test -run TestPickIPs ./...`
Expected: PASS（4 个测试通过）

- [ ] **Step 5: 提交**

```powershell
cd server; gofmt -w netinfo.go netinfo_test.go
git add server/netinfo.go server/netinfo_test.go
git commit -m "feat(server): add LAN IP detection with virtual-adapter filtering" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 3: server/main 接入 registry + 扩展 runUI 签名

此任务把 `ClientRegistry` 接入连接处理，并把 `runUI` 签名扩展为带 `ips`、`reg`（两平台同步改）。Windows 端此刻仍用旧 systray 托盘（Task 5 才替换），但保证可编译。

**Files:**
- Modify: `server/server.go`（`Server` 结构、`handle`、新增 `ipOf`）
- Modify: `server/main.go`（创建 registry、DetectIPs、传参）
- Modify: `server/tray_windows.go`（`runUI` 签名加参数）
- Modify: `server/tray_other.go`（`runUI` 签名加参数）
- Test: `server/server_test.go`

- [ ] **Step 1: 写失败测试**

Create `server/server_test.go`:

```go
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// drive a full hello→challenge→auth handshake over net.Pipe and assert the
// registry reflects connect then disconnect.
func TestHandleRegistersAndDeregisters(t *testing.T) {
	reg := NewClientRegistry()
	s := &Server{password: "pw", name: "srv", inj: newInjector(), reg: reg}

	cli, srvConn := net.Pipe()
	go s.handle(srvConn)
	defer cli.Close()
	cli.SetDeadline(time.Now().Add(3 * time.Second))

	enc := json.NewEncoder(cli)
	sc := bufio.NewScanner(cli)

	// hello
	if err := enc.Encode(map[string]any{"t": "hello", "name": "TestPhone"}); err != nil {
		t.Fatal(err)
	}
	// challenge
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
	// auth
	if err := enc.Encode(map[string]any{"t": "auth", "proof": deriveProof("pw", salt, nonce)}); err != nil {
		t.Fatal(err)
	}
	// auth_ok
	if !sc.Scan() {
		t.Fatal("no auth_ok")
	}

	if !waitFor(func() bool { return len(reg.Snapshot()) == 1 }) {
		t.Fatalf("registry not populated, got %d", len(reg.Snapshot()))
	}
	if snap := reg.Snapshot(); snap[0].Name != "TestPhone" {
		t.Errorf("name = %q, want TestPhone", snap[0].Name)
	}

	cli.Close()
	if !waitFor(func() bool { return len(reg.Snapshot()) == 0 }) {
		t.Errorf("registry not cleared after disconnect, got %d", len(reg.Snapshot()))
	}
}

func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}
```

- [ ] **Step 2: 运行测试，确认失败**

Run: `cd server; go test -run TestHandleRegisters ./...`
Expected: FAIL（`Server` 无 `reg` 字段 → 编译错误）

- [ ] **Step 3: 改 `Server` 结构与 `handle`**

In `server/server.go`, 把结构体加 `reg` 字段：

```go
type Server struct {
	password string
	name     string
	inj      Injector
	reg      *ClientRegistry
}
```

在 `handle` 里，找到认证成功后的这两行：

```go
	s.send(c, map[string]any{"t": "auth_ok", "server": s.name, "ver": "0.1"})
	log.Printf("client connected: %s (%s)", hello.Name, addr)
```

在其后插入注册逻辑：

```go
	if s.reg != nil {
		cid := s.reg.Register(hello.Name, ipOf(addr))
		defer s.reg.Deregister(cid)
	}
```

- [ ] **Step 4: 加 `ipOf` helper**

In `server/server.go`, 在文件末尾 `sanitizeName` 旁加：

```go
// ipOf returns the host part of a net.Addr, dropping the port.
func ipOf(a net.Addr) string {
	if h, _, err := net.SplitHostPort(a.String()); err == nil {
		return h
	}
	return a.String()
}
```

- [ ] **Step 5: main.go 创建 registry + DetectIPs + 传参**

In `server/main.go`, 把构造 `srv` 一行改为带 `reg`：

```go
	reg := NewClientRegistry()
	srv := &Server{password: *pass, name: *name, inj: newInjector(), reg: reg}
	defer srv.inj.Close()
```

把最后一行 `runUI(*notray, *pass, *port)` 改为：

```go
	ips := DetectIPs()
	runUI(*notray, *pass, *port, ips, reg)
```

- [ ] **Step 6: 扩展两平台 runUI 签名**

In `server/tray_other.go`, 改签名：

```go
func runUI(notray bool, pass string, port int, ips HostIPs, reg *ClientRegistry) { select {} }
```

In `server/tray_windows.go`, 改 `runUI` 签名（body 暂不变，仍用 systray）：

```go
func runUI(notray bool, pass string, port int, ips HostIPs, reg *ClientRegistry) {
```

（`ips`、`reg` 此刻未使用——Go 允许未使用的函数参数，可编译。）

- [ ] **Step 7: 运行测试 + 双平台编译，确认通过**

```powershell
cd server
go test -race -run TestHandleRegisters ./...
go build ./...
$env:GOOS="windows"; go build ./...; Remove-Item Env:GOOS
```
Expected: 测试 PASS；两次 `go build` 均无错误。
（在非 Windows 开发机把第二条换成 `$env:GOOS="darwin"; go build ./...` 验证 mac 编译。）

- [ ] **Step 8: 提交**

```powershell
cd server; gofmt -w server.go main.go tray_other.go tray_windows.go server_test.go
git add server/server.go server/main.go server/tray_other.go server/tray_windows.go server/server_test.go
git commit -m "feat(server): wire ClientRegistry into handle and runUI signature" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 4: 构建准备（manifest + build.ps1）

walk 需要嵌入应用清单启用 Common Controls v6 与 DPI 感知。本任务只加构建产物，不动 Go 代码。

**Files:**
- Create: `server/assets/manifest.xml`
- Modify: `server/build.ps1`
- Modify: `server/.gitignore`（若不存在则创建）

- [ ] **Step 1: 写应用清单**

Create `server/assets/manifest.xml`:

```xml
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="1.0.0.0" processorArchitecture="*" name="RemoteMouse" type="win32"/>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls"
        version="6.0.0.0" processorArchitecture="*"
        publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true</dpiAware>
    </windowsSettings>
  </application>
</assembly>
```

- [ ] **Step 2: 更新 build.ps1**

Replace `server/build.ps1` 全文：

```powershell
# Build the RemoteMouse Windows server -> rmserver.exe
# Usage: powershell -ExecutionPolicy Bypass -File build.ps1
$ErrorActionPreference = "Stop"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

# Embed application manifest (Common Controls v6 + DPI) and the app icon.
# rsrc.syso is auto-linked by `go build` when present in the package dir.
if (-not (Test-Path rsrc.syso)) {
    Write-Host "generating rsrc.syso (manifest + icon)..."
    go run github.com/akavel/rsrc@latest -manifest assets\manifest.xml -ico assets\tray.ico -o rsrc.syso
}

go build -ldflags "-H windowsgui" -o rmserver.exe .
Write-Host "built rmserver.exe"
Write-Host "run:  .\rmserver.exe -pass 1234            # window + tray"
Write-Host "      .\rmserver.exe -pass 1234 -notray    # console"
```

- [ ] **Step 3: 忽略生成物**

Ensure `server/.gitignore` 含以下行（文件不存在则创建，存在则追加缺失行）：

```
rmserver.exe
rsrc.syso
```

- [ ] **Step 4: 提交**

```powershell
git add server/assets/manifest.xml server/build.ps1 server/.gitignore
git commit -m "build(server): embed manifest+icon via rsrc in build.ps1" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 5: walk Windows UI（替换 systray）

把 Windows UI 从 systray 托盘换成 walk 主窗口 + 托盘。原子替换以保证可编译：删除 `tray_windows.go`，新建 `autostart_windows.go` 与 `ui_windows.go`，并把 `tray_other.go` 重命名为 `ui_other.go`，切换 go.mod 依赖。GUI 无单测，靠编译 + 手动验收。

**Files:**
- Delete: `server/tray_windows.go`
- Create: `server/autostart_windows.go`
- Create: `server/ui_windows.go`
- Rename: `server/tray_other.go` → `server/ui_other.go`
- Modify: `server/go.mod` / `server/go.sum`（移除 systray，加 walk/win）

- [ ] **Step 1: 拆出自启逻辑**

Create `server/autostart_windows.go`:

```go
//go:build windows

package main

import (
	"os"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runVal = "RemoteMouse"

func autostartCmd(pass string, port int) string {
	exe, _ := os.Executable()
	return `"` + exe + `" -pass ` + pass + " -port " + strconv.Itoa(port)
}

func autostartOn() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runVal)
	return err == nil
}

func setAutostart(on bool, pass string, port int) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if on {
		k.SetStringValue(runVal, autostartCmd(pass, port))
	} else {
		k.DeleteValue(runVal)
	}
}
```

- [ ] **Step 2: 删除旧托盘文件**

```powershell
git rm server/tray_windows.go
```

（其 autostart 已移入 Step 1，其 `runUI` 由 Step 4 的 walk 版本取代。）

- [ ] **Step 3: 重命名 other 平台文件**

```powershell
git mv server/tray_other.go server/ui_other.go
```

内容已在 Task 3 改为新签名，无需再动。

- [ ] **Step 4: 写 walk UI**

Create `server/ui_windows.go`:

```go
//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

//go:embed assets/tray.ico
var trayICO []byte

// deviceModel feeds the connected-devices TableView from a ClientRegistry.
type deviceModel struct {
	walk.TableModelBase
	reg   *ClientRegistry
	items []Client
}

func (m *deviceModel) RowCount() int { return len(m.items) }

func (m *deviceModel) Value(row, col int) interface{} {
	c := m.items[row]
	switch col {
	case 0:
		if c.Name == "" {
			return "(未命名)"
		}
		return c.Name
	case 1:
		return c.Addr
	case 2:
		return fmtDur(time.Since(c.Since))
	default:
		return "在线"
	}
}

func (m *deviceModel) reload() {
	m.items = m.reg.Snapshot()
	m.PublishRowsReset()
}

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
}

// loadIcon writes the embedded .ico to a temp file and loads it as a walk.Icon.
func loadIcon() *walk.Icon {
	tmp := filepath.Join(os.TempDir(), "rmserver_tray.ico")
	if err := os.WriteFile(tmp, trayICO, 0o644); err != nil {
		return nil
	}
	ic, err := walk.NewIconFromFile(tmp)
	if err != nil {
		return nil
	}
	return ic
}

func runUI(notray bool, pass string, port int, ips HostIPs, reg *ClientRegistry) {
	if notray {
		select {}
	}
	runtime.LockOSThread() // walk GUI must own its OS thread

	primary := ips.Primary
	if primary == "" {
		primary = "未检测到局域网 IP"
	}
	secText := strings.Join(ips.Secondary, "   ")

	var mw *walk.MainWindow
	var devCount *walk.Label
	var secLabel *walk.Label
	model := &deviceModel{reg: reg}

	err := MainWindow{
		AssignTo: &mw,
		Title:    "RemoteMouse",
		MinSize:  Size{Width: 440, Height: 380},
		Size:     Size{Width: 460, Height: 420},
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
					Label{Text: strconv.Itoa(port)},
					PushButton{Text: "复制", OnClicked: func() { copyClip(strconv.Itoa(port)) }},

					Label{Text: "密码"},
					Label{Text: pass},
					PushButton{Text: "复制", OnClicked: func() { copyClip(pass) }},

					Label{Text: "其他地址"},
					Label{AssignTo: &secLabel, Text: secText, Visible: false},
					PushButton{
						Text:    "展开",
						Enabled: secText != "",
						OnClicked: func() {
							secLabel.SetVisible(!secLabel.Visible())
						},
					},
				},
			},
			Label{AssignTo: &devCount, Text: "已连接设备 (0)"},
			TableView{
				MinSize: Size{Height: 170},
				Columns: []TableViewColumn{
					{Title: "设备名", Width: 130},
					{Title: "IP", Width: 130},
					{Title: "时长", Width: 80},
					{Title: "状态", Width: 60},
				},
				Model: model,
			},
		},
	}.Create()
	if err != nil {
		log.Printf("UI init failed, running headless: %v", err)
		select {}
	}

	if ic := loadIcon(); ic != nil {
		mw.SetIcon(ic)
	}

	refresh := func() {
		model.reload()
		devCount.SetText(fmt.Sprintf("已连接设备 (%d)", model.RowCount()))
	}
	reg.SetOnChange(func() { mw.Synchronize(refresh) })
	refresh()

	// per-second duration tick
	ticker := time.NewTicker(time.Second)
	go func() {
		for range ticker.C {
			mw.Synchronize(func() { model.PublishRowsReset() })
		}
	}()

	if ni, e := walk.NewNotifyIcon(mw); e == nil {
		defer ni.Dispose()
		if ic := loadIcon(); ic != nil {
			ni.SetIcon(ic)
		}
		ni.SetToolTip("RemoteMouse server")
		ni.SetVisible(true)

		show := walk.NewAction()
		show.SetText("显示窗口")
		show.Triggered().Attach(func() { mw.Show() })
		ni.ContextMenu().Actions().Add(show)

		auto := walk.NewAction()
		auto.SetText("开机自启")
		auto.SetCheckable(true)
		auto.SetChecked(autostartOn())
		auto.Triggered().Attach(func() {
			on := !auto.Checked()
			setAutostart(on, pass, port)
			auto.SetChecked(on)
		})
		ni.ContextMenu().Actions().Add(auto)

		quit := walk.NewAction()
		quit.SetText("退出")
		quit.Triggered().Attach(func() { walk.App().Exit(0) })
		ni.ContextMenu().Actions().Add(quit)

		ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
			if button == walk.LeftButton {
				if mw.Visible() {
					mw.Hide()
				} else {
					mw.Show()
				}
			}
		})
	} else {
		log.Printf("tray init failed: %v", e)
	}

	// close (X) hides to tray instead of exiting
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		*canceled = true
		mw.Hide()
	})

	mw.Show()
	mw.Run()
	ticker.Stop()
}

func copyClip(s string) {
	if err := walk.Clipboard().SetText(s); err != nil {
		log.Printf("clipboard: %v", err)
	}
}
```

- [ ] **Step 5: 切换依赖**

```powershell
cd server
go get github.com/lxn/walk@latest
go get github.com/lxn/win@latest
go mod tidy
```
`go mod tidy` 会移除不再被 import 的 `getlantern/systray` 及其间接依赖。

- [ ] **Step 6: 编译 Windows 目标，按编译器修正**

Run: `cd server; .\build.ps1`
Expected: 生成 `rsrc.syso` 与 `rmserver.exe`，无编译错误。

> 若 walk API 因版本不同报错，按编译器提示修正（最可能的两处）：
> - `walk.NewNotifyIcon`：旧版本无参，改 `walk.NewNotifyIcon()`；新版本需 `walk.NewNotifyIcon(mw)`（计划用后者）。
> - `Value(row, col int) interface{}`：若该 walk 版本要求 `any`，二者等价，无需改。
> 不要改动行为，仅对齐签名。

- [ ] **Step 7: 跨平台编译回归**

```powershell
cd server
go vet ./...
$env:GOOS="windows"; go build ./...; Remove-Item Env:GOOS
go test ./...
```
Expected: `go vet` 干净；Windows 构建通过；Task 1–3 的测试仍 PASS。
（非 Windows 开发机另跑 `$env:GOOS="darwin"; go build ./...` 确认 mac 不回归。）

- [ ] **Step 8: 手动验收（真机）**

启动 `.\rmserver.exe -pass 1234`，逐项确认：
1. 主窗口出现，推荐 IP/端口/密码正确，推荐 IP 加粗。
2. 三个「复制」按钮分别把 IP/端口/密码写入剪贴板（粘贴验证）。
3. 「展开」列出次要 IP；无次要 IP 时按钮禁用。
4. iOS 连接后设备实时出现（名/IP/状态），时长每秒走动；断开后实时消失，标题计数更新。
5. 点 × 窗口隐藏到托盘，server 仍在线；托盘左键单击切换显隐；右键菜单「显示窗口」可恢复。
6. 托盘「开机自启」勾选写注册表、取消删除；「退出」真正结束进程。
7. `.\rmserver.exe -pass 1234 -notray` 纯后台无窗口无托盘，控制面正常。

- [ ] **Step 9: 提交**

```powershell
cd server; gofmt -w ui_windows.go autostart_windows.go
# tray_windows.go 删除(Step 2)与 tray_other.go→ui_other.go 重命名(Step 3) 已 staged
git add server/ui_windows.go server/autostart_windows.go server/ui_other.go server/go.mod server/go.sum
git commit -m "feat(server): walk-based main window + tray showing IP/port/pass and live devices" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 6: 收尾（文档 + 全量校验）

**Files:**
- Modify: `server/README.md`
- Modify: `docs/12-progress.md`

- [ ] **Step 1: 更新 server/README.md**

在「运行 Windows」一节，把托盘描述更新为窗口+托盘。Replace 这段：

```
.\rmserver.exe -pass 1234            # 托盘图标：显示密码/端口、开机自启、退出
.\rmserver.exe -pass 1234 -notray    # 无托盘控制台模式
```

为：

```
.\rmserver.exe -pass 1234            # 主窗口 + 托盘：推荐IP/端口/密码 + 实时设备列表；关窗到托盘
.\rmserver.exe -pass 1234 -notray    # 纯后台控制台模式（无窗口无托盘）
```

并在「现状」一节追加一句：

```
Windows 端带可视化主窗口：智能高亮推荐局域网 IP（多网卡环境过滤虚拟网卡）、一键复制密码/端口、实时显示在线设备（设备名/IP/连接时长）。
```

- [ ] **Step 2: 更新 docs/12-progress.md**

在「已实现」的 Server 条目追加：

```
Windows 增可视化主窗口（walk）：推荐 IP 高亮+复制、端口/密码展示、在线设备实时列表；托盘统一显隐开关，关窗到托盘。
```

把「下一步」里 M3 收尾项中已完成的 UI 部分勾掉或标注（保持文档诚实）。

- [ ] **Step 3: 全量校验**

```powershell
cd server
gofmt -l .
go vet ./...
go test -race ./...
$env:GOOS="windows"; go build ./...; Remove-Item Env:GOOS
```
Expected: `gofmt -l .` 无输出（无未格式化文件）；`go vet` 干净；测试 PASS；Windows 构建通过。

- [ ] **Step 4: 提交**

```powershell
git add server/README.md docs/12-progress.md
git commit -m "docs: document Windows server visual UI" -m "Co-authored-by: Copilot App <223556219+Copilot@users.noreply.github.com>"
```

---

## 完成定义

- Task 1–3 单测/集成测试通过（`go test -race ./...`）。
- `go vet` 干净、`gofmt -l .` 无输出。
- `build.ps1` 产出 `rmserver.exe`；mac/windows 双 `go build` 通过。
- 手动验收清单（Task 5 Step 8）全部通过。
- `getlantern/systray` 已从 go.mod 移除；新增 `lxn/walk` + `lxn/win`。
- README 与 12-progress 文档已更新。
