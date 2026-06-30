# Windows Server 可视化 UI — 设计

> 状态：已定方向，待用户复审。目标：把 Windows server 从「仅托盘菜单」升级为带主窗口的可视化界面，展示本机连接信息（推荐 IP/端口/密码）与实时在线设备列表，托盘作为开关入口。

## 范围
- ✅ 独立桌面**主窗口**：展示推荐 IP（高亮+一键复制）、端口、密码、其余次要 IP；实时在线设备列表（设备名/IP/连接时长/状态）。
- ✅ 托盘入口：单击切换窗口显隐；右键菜单 = 显示窗口 / 开机自启 / 退出。关窗(×)隐藏到托盘，不退出。
- ✅ 新增跨平台**连接状态中枢** `ClientRegistry`，server 与 UI 解耦；新增**本机 IP 探测** `netinfo`。
- ✅ UI 技术栈用 `lxn/walk`（原生 Win32，cgo-free），窗口+托盘统一一套框架，移除 `getlantern/systray`。
- ❌ 不做：UI 内修改密码/端口（仍走命令行参数，只读展示+复制）；设备「断开/踢出」操作；连接历史/日志；mac/other 平台 UI（保持现状无窗口）；TLS/UDP、锁屏服务（后续里程碑）。

## 1. 整体架构
GUI 仅 Windows；mac=CGEvent 注入、other=日志桩、非 Windows 无窗口，全部不变。

```
main.go ── 创建 ClientRegistry + 探测 HostIPs，注入 Server 和 runUI
  ├─ Server.Listen        (goroutine)        控制面 TCP
  ├─ ClientRegistry       (跨平台)           连接状态中枢：Add/Remove/Snapshot + 变更订阅
  └─ runUI(...)           (主 goroutine，平台分文件)
       ├─ Windows → walk 主窗口 + NotifyIcon 托盘   (ui_windows.go)
       └─ other   → select{} 阻塞（无 UI）          (ui_other.go)
```

## 2. 文件改动（沿用 build-tag 分平台风格）
| 文件 | 变化 | 说明 |
|------|------|------|
| `clients.go` | 🆕 跨平台 | `ClientRegistry`：线程安全在线连接表 + 订阅回调 |
| `netinfo.go` | 🆕 跨平台 | 枚举网卡，推荐局域网 IP + 次要 IP 列表 |
| `server.go` | ✏️ | `Server` 增 `reg *ClientRegistry`；`handle` 认证成功后 `Register`，`defer Deregister` |
| `main.go` | ✏️ | 创建 `ClientRegistry`、调用 `DetectIPs()`，串入 `Server` 与 `runUI` |
| `ui_windows.go` | 🆕（替换 `tray_windows.go` 的 UI 部分）| walk 主窗口 + `NotifyIcon` 托盘 |
| `autostart_windows.go` | 🆕（从 `tray_windows.go` 拆出）| 注册表开机自启逻辑原样保留 |
| `ui_other.go` | ♻️（原 `tray_other.go` 重命名）| `runUI` = `select{}` 不变 |
| `assets/manifest.xml` | 🆕 | 应用清单：Common Controls v6 + DPI 感知 |
| `build.ps1` | ✏️ | 无 `rsrc.syso` 则用 `rsrc` 生成后再 `go build` |

依赖变化：移除 `github.com/getlantern/systray`；新增 `github.com/lxn/walk` + `github.com/lxn/win`（均 cgo-free）。连接表命名 `ClientRegistry`，避免与自启用到的 `x/sys/windows/registry` 包重名。

## 3. 连接状态中枢 `clients.go`（跨平台）
```go
type Client struct {
    Name  string    // hello.Name，设备名；空则 UI 显示「(未命名)」
    Addr  string    // 远端 IP（取 RemoteAddr 的 host 部分，去端口）
    Since time.Time // 认证成功时刻 → 算连接时长
}

type ClientRegistry struct {
    mu       sync.RWMutex
    clients  map[uint64]*Client
    nextID   uint64
    onChange func()       // UI 订阅；可空
}

func NewClientRegistry() *ClientRegistry
func (r *ClientRegistry) Register(name, addr string) uint64 // 返回 id，触发 onChange
func (r *ClientRegistry) Deregister(id uint64)              // 触发 onChange
func (r *ClientRegistry) Snapshot() []Client               // 拷贝，按 Since 升序
func (r *ClientRegistry) SetOnChange(f func())             // UI 注册回调
```
- `server.handle` 认证通过（发出 `auth_ok` 后）：`id := s.reg.Register(hello.Name, ipOf(addr))`；`defer s.reg.Deregister(id)`。
- `onChange` 在网络 goroutine 触发；UI 侧回调内用 walk `Synchronize` 切回 GUI 线程刷新，禁止跨线程直接操作控件。
- `reg` 可为 `nil`（非 Windows / 未接 UI），server 调用前判空或注入空实现。

## 4. 本机 IP 探测 `netinfo.go`（跨平台）
```go
type HostIPs struct {
    Primary   string   // 推荐 IP（活动物理网卡私网 IPv4）；无则空
    Secondary []string // 其余可用 IPv4
}
func DetectIPs() HostIPs
```
算法（启发式）：
1. `net.Interfaces()` 枚举，跳过 `FlagUp==0`、`FlagLoopback` 的接口。
2. 取每个接口的 IPv4，排除回环、APIPA `169.254.0.0/16`。
3. 接口名/描述含虚拟网卡关键词（`vEthernet`、`Hyper-V`、`VMware`、`VirtualBox`、`VBox`、`Loopback`、`Bluetooth`、`WSL`、`Tailscale`、`docker`）的归为次要。
4. 私网段（`10/8`、`172.16/12`、`192.168/16`）的**物理**接口 IPv4 优先，取首个为 `Primary`，其余进 `Secondary`。
5. 选不出 `Primary` 时回退：首个非回环 IPv4。仍无则 `Primary` 留空。

可测性：把「`[]net.Interface`+地址 → `HostIPs`」的纯函数 `pickIPs(...)` 抽出，单测喂构造数据，不依赖真机网卡。

## 5. Windows GUI `ui_windows.go`（walk）
窗口布局：
```
┌─ RemoteMouse ────────────────────────────┐
│ 连接信息                                    │
│   推荐 IP  10.32.86.111   [复制]  ★高亮     │
│   端口     27500          [复制]            │
│   密码     1234           [复制]            │
│   其他地址 10.95.140.97…  [▼展开]           │
│ ───────────────────────────────────────── │
│ 已连接设备 (1)                              │
│   设备名      │ IP          │ 时长    │ 状态 │  ← TableView
│   我的 iPhone │ 10.95.140.20│ 00:03:12│ 在线 │
└───────────────────────────────────────────┘
```
- **主窗口**：`walk.MainWindow`。信息区用 `Label`+`PushButton`（复制→`walk.Clipboard().SetText`）。推荐 IP 用加粗/醒目字体。次要 IP 默认折叠，「展开」按钮切换可见。
- **设备列表**：`walk.TableView` + 自定义 `TableModel`（列：设备名/IP/时长/状态）。数据源 = `ClientRegistry.Snapshot()`。
- **实时刷新**：`reg.SetOnChange(func(){ mw.Synchronize(refresh) })`，`refresh` 重新拉 `Snapshot()` 并 `PublishRowsReset()`，更新标题「已连接设备 (N)」。
- **时长走动**：`time.Ticker(1s)` goroutine → `mw.Synchronize`，对列表「时长」列做 `PublishRowsReset()`/局部刷新；时长 = `now - Since` 格式 `HH:MM:SS`。
- **托盘**：`walk.NotifyIcon`，设 `assets/tray.ico` 图标 + tooltip。`MouseDown`（左键）切换窗口显隐；右键 `ContextMenu` 三项：显示窗口 / 开机自启（勾选态读写 `autostart_windows.go`）/ 退出。
- **关窗到托盘**：拦截 `mw.Closing()`，`*canceled = true; mw.SetVisible(false)`；仅托盘「退出」调用 `walk.App().Exit(0)`。
- `-notray`：不初始化 walk，纯后台（同现状）。

## 6. 错误处理与边界
- walk 主窗口/托盘**初始化失败**：记日志并回退到 `select{}` 纯后台，控制面照常工作，server 不崩。
- `onChange` 跨线程：一律经 `Synchronize`；回调内不阻塞。
- IP 探测无结果：推荐 IP 显示「未检测到局域网 IP」，仍列出 `Secondary`（若有）。
- 设备名为空：显示「(未命名)」。
- 剪贴板复制失败：静默 + 日志，不打断。

## 7. 构建与应用清单
- walk 需嵌入清单启用 Common Controls v6（否则控件为旧外观）与 DPI 感知。
- `assets/manifest.xml` → `github.com/akavel/rsrc` 生成 `rsrc.syso`（与 `.go` 同目录，`go build` 自动链接）。
- `build.ps1`：若缺 `rsrc.syso` 则 `go run github.com/akavel/rsrc@latest -manifest assets/manifest.xml -ico assets/tray.ico -o rsrc.syso`，再 `go build -ldflags "-H windowsgui" -o rmserver.exe .`。
- 复用 `assets/tray.ico` 作窗口与托盘图标。

## 8. 测试策略
- `netinfo_test.go`：构造接口/地址数据喂 `pickIPs`，验证虚拟网卡过滤、APIPA 排除、私网优先、回退路径。
- `clients_test.go`：并发 `Register`/`Deregister`/`Snapshot` 线程安全；`onChange` 触发计数；`Snapshot` 排序与隔离（返回拷贝）。
- GUI 不做自动化 → **手动验收清单**（见下）。
- 回归：现有 `keys_test`/`keys_windows_test` 通过；`gofmt`/`go vet` 干净；mac、windows 双 `go build` 通过。

## 9. 验收清单（手动）
1. `rmserver.exe` 启动出现主窗口，显示正确的推荐 IP、端口、密码；推荐 IP 高亮。
2. 「复制」按钮可把 IP/端口/密码写入剪贴板。
3. 「其他地址」展开列出次要 IP。
4. iOS 连接后，设备列表实时出现该设备（名/IP/状态），时长每秒走动；断开后实时消失，标题计数更新。
5. 点 × 窗口隐藏到托盘，server 仍在线；托盘单击重新显示窗口。
6. 托盘右键：显示窗口 / 开机自启（勾选写注册表、取消删除）/ 退出（真正结束进程）。
7. `-notray` 纯后台无窗口无托盘，控制面正常。
8. 多网卡环境推荐 IP 选中物理活动网卡的私网地址（非 vEthernet/蓝牙等）。
