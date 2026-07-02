# 12 — 当前进度

> 单一事实来源：每次迭代结束更新此页，新会话先读这里。里程碑定义见 [10-roadmap](10-roadmap.md)。

**更新时间**：2026-07-02 ｜ **状态**：M1+M2 完成；M3 部分完成（Windows server 真机注入 + 键盘/快捷/媒体映射 + 集成控制面板（装/卸/启停服务、启动类型、热改端口/密码/设备名、端口仲裁、服务模式设备列表）+ 自启，iOS 按键栏）；M5 锁屏注入 POC（Windows SYSTEM 服务 + 按桌面 agent，iPhone 打 Hello PIN 解锁）

## 已实现
- **协议** `proto/protocol-mvp.md`：单 TCP + 换行 JSON；PBKDF2-HMAC 挑战-响应认证；move/click/button/scroll/text/key/ping。key 含方向/编辑/F键/快捷键/媒体。
- **Server** `server/`（Go）：mDNS 广播 + TCP 监听 + 认证 + 注入抽象。mac=CGEvent、Windows=SendInput（已修尺寸/批量/键盘/媒体，本机真机跑通）、其它=日志桩。Windows 用 `lxn/walk` 原生窗口 + 托盘 UI：展示推荐 IP/端口/密码（一键复制）与实时在线设备列表（`ClientRegistry` 连接中枢 + `netinfo` IP 探测），注册表开机自启。**集成控制面板**（`ui_panel_windows.go`）：单窗口装/卸/启停服务、设启动类型（自动/手动/禁用＝服务开机自启）、热改端口/密码/设备名并即时 Apply（`servingController` in `serving_windows.go`）、`ServingArbiter` 在服务与 UI 间仲裁 TCP 端口、服务模式经状态命名管道（`status_windows.go`）约 1 秒刷新在线设备；管理操作走 `elevatedSelf` 一次性 UAC 提权；新增 CLI 模式 `start/stop/set-start-type/apply-config`。`build.ps1`（嵌入清单+图标）出 rmserver.exe，`-notray` 纯后台、`-standalone` 旧版简易窗口。（详见 `docs/superpowers/plans/2026-07-01-control-panel.md`，13 Task 全部完成）
- **iOS** `ios/`（SwiftUI，xcodegen）：Bonjour 发现、密码/手动 IP、触控板 + 按键栏(方向/回退/快捷/媒体)。
- **锁屏注入（M5 POC）** `server/`（Windows）：单 `rmserver.exe` 三模式——默认托盘 UI（读 HKLM 配置 + 安装/启停服务）、`-service`（LocalSystem 持 TCP+鉴权+事件解析，命名管道转发，桌面监控按 `Default`/`Winlogon` 跟随重开 agent）、`-agent`（`CreateProcessAsUser` 拉到当前输入桌面，复用 `SendInput`）。锁屏从 iPhone 输入 Hello PIN 解锁。日志写 `%ProgramData%\RemoteMouse`。

## 验证
- 服务端握手/认证/ping、mDNS 广播 ✅
- Swift PBKDF2+HMAC 与 Go 逐字节一致 ✅
- Windows：go test 通过、smoke 客户端注入 move/text/key/快捷键无误、rmserver.exe 监听 ✅
- ⚠️ mac key 注入与 iOS 按键栏未在 mac 实测；Windows 媒体键/中文长测待补

## 已知缺口
- 无 TLS / 数据面仍走 TCP（未上 UDP+AEAD）。iOS 端已开 TCP_NODELAY + 客户端 ~125Hz 移动合并节流；服务端仍无移动事件速率上限。
- iOS：仅 1234 默认密码、无记住设备、无重连；中文/特殊键未充分测。
- 输入体验：双指/拖拽/灵敏度配置未完成；mac 键盘与媒体键仍需真机验证。

## 下一步（建议顺序）
1. 收尾 M3：mac key 真机验证、Windows 媒体/中文长测、双指/拖拽/灵敏度。（移动节流：iOS 客户端合并已完成，待 mac 真机手感验证）
2. 安全与传输：UDP 数据面 + 加密、TLS、认证失败限速。
3. M4 体验：记住设备、重连、受信设备免密。
4. 锁屏（Win SYSTEM 服务）：POC 代码完成（install/start → 解锁态注入 → Win+L 检测 Winlogon → 打 PIN 解锁），**待真机端到端验证**。收尾：TLS（PIN 明文 LAN 风险）、acceptWithTimeout 超时后的孤儿连接、WTS 通知替代轮询、管道/HKLM ACL 收紧、服务签名+安装包。

## 续接清单
README → proto/protocol-mvp.md → 本页 → 选下一里程碑。
