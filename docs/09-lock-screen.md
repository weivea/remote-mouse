# 09 — 锁屏控制可行性（关键难点）

> 这是全项目最难、风险最高的需求。结论：**Windows 可行，macOS 纯软件基本不可行**。建议作为拉伸目标，分平台分级实现。

## 为什么难
锁屏/登录界面运行在受保护的安全桌面（Win secure desktop / mac loginwindow），系统**设计上禁止**普通进程注入键鼠，防止恶意软件偷输密码。

## Windows — 可行 ✅
原理同 TeamViewer/AnyDesk：
1. 装一个 **Windows 服务**，以 **SYSTEM** 运行。
2. 检测安全桌面激活时，用 `OpenInputDesktop`/`SetThreadDesktop` 把线程切到当前输入桌面（Winlogon/Secure Desktop），再 `SendInput`。
3. 解锁可用标准 Credential Provider 流程或代输密码。
- 要求：管理员安装、服务常驻、CtrlAltDel/UAC 需服务级权限。
- 风险：实现复杂、需签名、不同 Win 版本差异。MVP 后做。

### 已实现（POC，2026-06-30）
单一 `rmserver.exe` 三模式：默认托盘 UI / `-service`（LocalSystem，持 TCP+鉴权+事件解析+桌面监控）/ `-agent`（被服务用 `CreateProcessAsUser` 拉到当前输入桌面，复用 `SendInput`）。服务经命名管道把事件转发给 agent；锁屏时检测到 `Winlogon` 桌面即用 winlogon token 在安全桌面重开 agent，从 iPhone 输入 Hello PIN 解锁。设计与计划见 `docs/superpowers/specs/2026-06-30-windows-lock-screen-design.md`、`docs/superpowers/plans/2026-06-30-windows-lock-screen.md`。

安装（管理员 PowerShell）：

```powershell
.\rmserver.exe -install-service -pass <密码> -port 27500   # 安装并启动
.\rmserver.exe -uninstall-service                          # 停止并卸载
```

日志：`C:\ProgramData\RemoteMouse\service.log`、`agent.log`（按键内容脱敏，不记录 PIN）。
仍是 POC：PIN 走明文 LAN，仅限可信网络；TLS 为紧随其后的下一步。

### 真机验证清单（管理员 PowerShell）
锁屏注入必须以 LocalSystem 服务运行，需管理员权限，且只能真机验证（子代理/CI 无法自动化）。

1. 构建：`cd server; .\build.ps1` 出 `rmserver.exe`。
2. 安装并启动：`.\rmserver.exe -install-service -pass 1234 -port 27500`（自动启动）；`sc.exe query RemoteMouse` 期望 `STATE: 4 RUNNING`。
3. 查 `service.log`，应依次出现 `service log opened`、`config: port=27500 (password loaded from HKLM)`、`mDNS announce: ...`、`agent bound to session=N desktop="Default"`。
4. 解锁态验证控制：iPhone 连接（自动发现或手填 `本机IP:27500`，密码 1234）→ 移动/点击/滚动/打字均正常。
5. 锁屏验证：`Win+L` → service.log 出现 `desktop change -> ... desktop="Winlogon"` 然后 `agent bound to ... desktop="Winlogon"` → iPhone 输入 Hello PIN → 解锁。
6. 解锁后：service.log 回到 `desktop="Default"`，控制自动跟回。
7. 卸载：`.\rmserver.exe -uninstall-service`（自动停止 + 删除）。

排障：不解锁时看 `service.log` / `agent.log`（`agent connected; injecting on this desktop` 表示 agent 已在该桌面就绪），日志不记录 PIN 内容；最可能的失败点是 winlogon token 获取或安全桌面上的 agent spawn。

## macOS — 基本不可行 ❌（纯软件）
- 登录窗/锁屏由 root 的 WindowServer 托管，`CGEventPost`/Accessibility **无法**注入，root 的 LaunchDaemon 也在 session 0 够不到。
- ARD/VNC 也只能在登录后控制。SIP/TCC 封死，无官方旁路。
可选退路：
1. **降级**：仅已登录会话控制（推荐先做）。
2. 软件唤醒+控制，解锁交给系统（Apple Watch/Touch ID）。
3. **硬件 HID 桥**：手机→WiFi/BLE→ESP32/Pi Pico→USB 键鼠，对 PC 像真键盘，可解锁、OS 无关（最稳但要硬件）。

## 手机直连 BT HID？
- iOS 禁止三方做 HID device；Android 受限（多需 root）。不靠谱，故用 USB 微控制器桥。

## 分级建议
| 级别 | 能力 | 平台 |
|------|------|------|
| L0 | 已登录控制 | Win+Mac（MVP）|
| L1 | 锁屏控制+解锁 | Windows 服务 |
| L2 | 锁屏解锁 | macOS 仅硬件桥 |

## 待办
- Win secure-desktop 注入 POC；mac 现状复核；评估 ESP32 HID 桥。详见 [06](06-server.md)/[08](08-security.md)/[10](10-roadmap.md)。
