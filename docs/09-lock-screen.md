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
