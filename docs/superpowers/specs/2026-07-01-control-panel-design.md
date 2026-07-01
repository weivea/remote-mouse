# rmserver 控制面板设计

- 日期：2026-07-01
- 状态：已确认（待实现）
- 平台：Windows（`//go:build windows`），非 Windows 保持现有桩行为
- 相关代码：`server/`（Go）

## 1. 背景与问题

当前 `rmserver.exe` 的服务管理只能通过命令行完成：

```
.\rmserver.exe -install-service -pass 1234 -port 27500   # 安装
.\rmserver.exe -uninstall-service                         # 卸载
net start/stop RemoteMouse                                # 启停
```

主窗口（`ui_windows.go`）已展示连接信息（IP/端口/密码）与在线设备表；托盘右键菜单里塞了只读的「服务状态」和「安装/卸载服务」项，功能零散、发现性差，且**不能改端口/密码**。

用户希望：打开一个**控制面板**即可完成 安装/卸载/启停服务、设置服务开机自启，并**实时展示服务状态与连接状态（IP/端口/密码）**，同时**端口与密码可动态修改**。

## 2. 目标与非目标

### 目标
1. 把现有主窗口升级为完整控制面板（**方案 A：整合进主窗口**，不新开窗口）。
2. 「服务管理」区：实时状态 + 启动类型（自动/手动/禁用）下拉 + 安装并启动/启动/停止/卸载 按钮。
3. 「连接信息」区：端口、密码、设备名**可编辑**；IP 只读+复制；密码可显示/隐藏切换。
4. 「保存并应用」**自动重启**生效（服务模式重启服务；独立模式进程内热重配）。
5. 服务模式下也能看到在线设备列表（经服务→UI 的 IPC 状态管道）。
6. 端口归属**动态仲裁**，消除"UI 独立 serving 时启动服务会端口冲突"的隐患。

### 非目标
- 不改 iOS 客户端；不改协议。
- 不做密码加密存储（HKLM 明文是既有 TODO，本设计不处理）。
- 不做多语言；沿用中文界面。
- 不新增打包/安装器。

## 3. 已确认决策

| # | 决策 | 结论 |
|---|------|------|
| D1 | 面板位置 | **整合进现有主窗口**（不新开窗口） |
| D2 | 可编辑字段 | 端口、密码、设备名可编辑；IP 只读 |
| D3 | 保存行为 | **保存并应用 = 自动重启/热重配** |
| D4 | 权限模型 | **每操作 UAC 提权**（沿用 `elevatedSelf`），UI 本身非管理员；多步操作合并成一次提权 |
| D5 | 端口仲裁 | UI 内置 **ServingArbiter**，跟随服务状态动态释放/接管端口 |
| D6 | 服务模式设备列表 | **做**，经 `\\.\pipe\remotemouse-status` IPC 查询（只传设备元数据，不传密码） |
| D7 | 开机自启语义 | 面板「启动类型=自动」即**服务开机自启**；原托盘 HKCU 自启项重命名为「登录时打开界面」以消歧义 |

## 4. 架构总览

`rmserver.exe`（UI 模式）打开即控制面板。三个协作单元，各自单一职责、边界清晰、可独立测试：

- **StatusTicker（约 1s）**：后台 goroutine，查询 SCM（服务状态 + 启动类型），若为服务模式再经 IPC 拉取设备快照；通过 `mw.Synchronize` 回到 GUI 线程刷新控件与按钮启停用。**绝不在 GUI 线程上做管道/SCM 阻塞 I/O。**
- **ServingArbiter**：依据服务状态仲裁本进程是否 serving（占用端口）。纯决策函数 `wantStandalone(running bool) bool` 便于单测；副作用（Start/Stop）委托给 servingController。
- **servingController**：封装独立模式的 serving（`Server` + `net.Listener` + zeroconf mDNS）。方法 `Start()`/`Stop()`（接管/释放端口）、`Apply(cfg)`（热重配）。

管理动作（安装/卸载/启停/改启动类型/写 HKLM）通过 `elevatedSelf(flag)` 拉起一个**提权子进程**完成，UI 只需轮询状态感知结果。

### 4.1 ServingArbiter 状态机

```
             服务运行中
   [检测] ───────────────► [控制器模式]  本地监听已停(让出 27500)，UI 当控制器
     │                          │  ▲
     │ 服务未运行                 │  │ 服务启动 → Stop()(释放端口)
     ▼                          ▼  │
 [独立模式] ◄───────────────────────┘
  本地 serving(占用 27500)   服务停止/卸载 → Start()(接管端口)
```

轮询驱动：每次 StatusTicker 拿到服务状态后调用 arbiter，`wantStandalone = !serviceRunning`，与 controller 当前 running 状态比对，必要时 Start/Stop。

### 4.2 服务 ↔ UI 状态 IPC

- 服务侧：`runServiceCore` 内起 goroutine `serveStatusPipe(name, reg, stop)`。基于 `github.com/Microsoft/go-winio` `ListenPipe`，每个连接回写 `reg.Snapshot()` 的 JSON（`[]Client`：Name/Addr/Since）后关闭。SDDL 允许交互用户（IU）连接——UI 以登录用户身份运行、非管理员。
- UI 侧（控制器模式）：`queryStatusPipe()` 在后台 `DialPipe`+读+解码，带超时；结果经 `Synchronize` 填入设备表。拨号失败（服务未就绪）则显示空列表+提示。
- 管道名常量 `statusPipe = \\.\pipe\remotemouse-status`，与注入管道 `-pipe` 独立。
- 安全：状态管道仅暴露设备名/IP/连接时长，**永不包含密码**。

## 5. UI 布局（整合进主窗口）

单窗口，VBox，三块 GroupBox：

1. **连接信息**（Grid 3 列）
   - 推荐 IP：只读加粗 + 「复制」
   - 端口：`LineEdit`（可编辑）+ 「复制」
   - 密码：`LineEdit`（可编辑，`PasswordMode` 可切换显示/隐藏 👁）+ 「复制」
   - 设备名：`LineEdit`（可编辑）
   - 「保存并应用」按钮 + 一行灰字提示（"服务在跑会自动重启生效，弹 UAC"）
2. **服务管理**
   - 状态：带颜色圆点的实时标签（未安装/已停止/运行中/启动中/停止中）+ 当前启动类型
   - 启动类型：下拉 `ComboBox`[自动/手动/禁用]（= 服务开机自启）
   - 按钮行：安装并启动 / 启动 / 停止 / 卸载（按状态启停用，见 §7 矩阵）
   - 一行灰字提示（"操作需要管理员，弹 UAC"）
3. **已连接设备 (N)** + `TableView`（设备名/IP/时长/状态），沿用现有 `deviceModel`

窗口关闭仍隐藏到托盘（现有行为）。托盘菜单精简：显示窗口 / 登录时打开界面（原「开机自启」重命名）/ 退出；原托盘内的服务相关项移除（已在主窗口）。

## 6. 配置与持久化模型

`serverConfig` 增加 `Name` 字段：

```go
type serverConfig struct {
    Password string
    Port     int
    Name     string // 设备/mDNS 名
}
```

- **服务模式**：配置存 HKLM（`SOFTWARE\RemoteMouse`）。服务 `runServiceCore` 读取 Password/Port/**Name**（Name 用于 mDNS 广播）。写 HKLM 需管理员。
- **独立模式**：配置即进程内 `Server` 的状态；不强制落盘。若「登录时打开界面」(HKCU Run) 开着，保存时同步改写其命令行（含新 pass/port/name，HKCU 无需管理员）。

## 7. 交互细节

### 7.1 服务按钮启停用矩阵

| 服务状态 | 可用按钮 | 启动类型下拉 |
|---------|---------|-------------|
| 未安装 | 安装并启动 | 禁用 |
| 已停止 | 启动、卸载 | 可用 |
| 运行中 | 停止、卸载 | 可用 |
| 启动中/停止中 | 全部暂禁用 | 禁用 |

### 7.2 「保存并应用」流程

规则拆成"持久化"与"应用到当前 serving 者"两件事，按当前状态三分支（消除"已安装但已停止"边界歧义）：

```
改端口/密码/设备名 → 保存并应用
        │
        ▼
   服务运行中?
   ├─ 是 → 提权一次 -apply-config: 写 HKLM + 停止→启动服务      [断开约 1s]
   │
   ├─ 否, 但服务已安装(停止) → 提权 -apply-config: 只写 HKLM(为下次启动持久化)
   │                          + UI 侧对当前独立 serving 做进程内热重配
   │
   └─ 否, 未安装(纯独立) → 仅进程内热重配, 无需提权:
                 · 密码热换(仅影响新认证, 现有连接不掉)
                 · 端口变化则重绑新 Listener(现有连接保留)
                 · 设备名/端口变化则 mDNS 重播
              → 若「登录时打开界面」开着: 改写 HKCU Run 命令(含新 pass/port/name)
```

其中 `-apply-config` 内部逻辑固定为"写 HKLM，若服务运行中则重启"，因此上面前两支复用同一提权入口（运行中重启、停止则不重启），差异只在 UI 是否再对本地 serving 做热重配。

输入校验：端口为 1–65535 整数；密码非空；非法输入弹提示、不应用。

### 7.3 启动类型下拉

切换下拉 → `elevatedSelf("-set-start-type", "auto|manual|disabled")`（一次 UAC）。成功后 StatusTicker 下一拍反映新值；失败（用户取消 UAC）则回滚下拉到查询值。

## 8. 服务操作与新增 CLI

新增 mode/flag（均经 `elevatedSelf` 提权执行、干完退出）：

| Flag | 行为 |
|------|------|
| `-start-service` | `SCM` 启动 RemoteMouse |
| `-stop-service` | `SCM` 停止 RemoteMouse |
| `-set-start-type auto\|manual\|disabled` | 设服务启动类型（`mgr.Config.StartType` / `SERVICE_DISABLED`） |
| `-apply-config -pass X -port Y -name Z` | 写 HKLM + 若服务运行中则重启 |

`main.go` 增补对应 `flag` 与 `appConfig` 字段；`run_windows.go` 的 `run()` 与 `mode()` 增补分支。既有 `-install-service`/`-uninstall-service` 保留；install 仍默认 `StartAutomatic`，后续由下拉调整。

`service_windows.go` 增补：`startService()`、`stopService()`、`restartService()`、`setStartType(t)`、`applyConfigElevated(cfg)`。`ui_service_windows.go` 增补 `serviceStartType() (startType, string)` 查询（只读 SCM，无需管理员）。

## 9. server.go 重构（支撑独立模式热重配）

- `Listen(port)` 拆分：新增 `Serve(ln net.Listener) error` 跑 Accept 循环；`Listen(port)` 变为"建 Listener 再 Serve"的便捷封装。这样 controller 持有 Listener 并可 `Close()` 重绑。
- 密码加锁：新增 `SetPassword(p string)` 与内部 `password()` getter（`sync.RWMutex` 或 `atomic.Pointer[string]`）；`handle()` 改用 getter，消除数据竞争，支持热换。

## 10. 模块/文件划分

| 文件 | 改动 |
|------|------|
| `config_windows.go` | `serverConfig` 加 `Name`；read/write 读写 Name |
| `server.go` | `Serve(ln)` 拆分；`SetPassword/password()` 加锁 |
| `serving_windows.go` **(新)** | `servingController`(Start/Stop/Apply) + 纯函数 `wantStandalone(running)` |
| `status_windows.go` **(新)** | `serveStatusPipe(reg)`(服务侧)、`queryStatusPipe()`(UI 侧)、快照 JSON 编解码、`statusPipe` 常量 |
| `service_windows.go` | start/stop/restart/setStartType/applyConfigElevated；`runServiceCore` 起状态管道 goroutine |
| `ui_service_windows.go` | `serviceStartType()` 查询 |
| `ui_windows.go`（可拆 `ui_panel_windows.go`） | 可编辑连接字段 + 服务管理 GroupBox + StatusTicker/Arbiter 接线；托盘菜单精简 |
| `autostart_windows.go` | Run 命令加 `-name`；提供改写辅助（保存配置时同步） |
| `main.go` / `run_windows.go` | 新 flag/mode 与分支 |

> 若 `ui_windows.go` 因新增控件明显变大，把服务管理与连接编辑相关的 widget 构造拆到 `ui_panel_windows.go`，保持单文件聚焦。

## 11. 错误处理

- 提权被取消（UAC 拒绝）：操作不发生，UI 下一拍轮询恢复真实状态；下拉类操作回滚显示。
- 状态管道拨号失败：设备表显示空 + 一行提示"服务模式下暂无法读取设备（服务可能刚启动）"，不报错弹窗。
- 独立模式端口重绑失败（端口被占）：弹提示保留旧监听，不使 UI 崩溃。
- SCM/注册表读取失败：状态显示"未知"，按钮保守禁用。
- 所有后台 I/O 有超时，避免卡 GUI 线程。

## 12. 测试计划

新增/扩展单元测试（GUI 本体沿用现状不测）：

- `config_windows_test.go`：`serverConfig`（含 Name）写入→读取往返。
- 启动类型枚举 ↔ SCM 值（`StartAutomatic/StartManual/SERVICE_DISABLED`）↔ 字符串（auto/manual/disabled）↔ 中文标签的双向映射。
- `status_windows_test.go`：`[]Client` 快照 JSON 编码→解码往返（含 Since 时间保真、排序）。
- `serving_windows_test.go`：`wantStandalone(running)` 真值表；配置 diff（端口是否变化 → 是否需重绑）判定。
- `main_test.go`：新 flag 解析与 `mode()` 分派（install/uninstall 优先级不受影响）。
- `server_test.go`：`SetPassword` 并发读写在 `-race` 下无竞争；换密码后新认证用新密码、旧连接不受影响。

`go build`（`GOOS=windows`）+ `go test ./...` 通过为完成基线。

## 13. 已知限制与后续

- 状态管道 SDDL 沿用 POC 的宽松 DACL（SY/BA/IU）；生产应收紧。
- HKLM 密码仍明文（既有 TODO，未处理）。
- 服务模式设备列表为 1s 轮询（非推送），有约 1s 延迟。
- 独立模式端口变更时，正在连接的手机保留在旧连接；新端口仅影响新连接与 mDNS——符合"几乎无感"，但客户端若缓存旧端口需重新发现。

## 14. 安全说明

- 状态 IPC 仅传设备名/IP/时长，**不含密码**。
- UI 进程保持非管理员；仅在明确的管理动作时按需 UAC 提权，最小化提权面。
- 复制到剪贴板的密码为用户显式点击行为，维持现状。
