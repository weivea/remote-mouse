# 12 — 当前进度

> 单一事实来源：每次迭代结束更新此页，新会话先读这里。里程碑定义见 [10-roadmap](10-roadmap.md)。

**更新时间**：2026-06-29 ｜ **状态**：M1+M2 完成；Windows server 真机注入跑通 + 全输入键盘 + 托盘自启

## 已实现
- **协议** `proto/protocol-mvp.md`：单 TCP + 换行 JSON；PBKDF2-HMAC 挑战-响应认证；move/click/button/scroll/text/key/ping。key 含方向/编辑/F键/快捷键/媒体。
- **Server** `server/`（Go）：mDNS 广播 + TCP 监听 + 认证 + 注入抽象。mac=CGEvent、Windows=SendInput（已修尺寸/批量/键盘/媒体，本机真机跑通）、其它=日志桩。Windows 托盘(systray)+注册表开机自启，`build.ps1` 出 rmserver.exe，`-notray` 控制台。
- **iOS** `ios/`（SwiftUI，xcodegen）：Bonjour 发现、密码/手动 IP、触控板 + 按键栏(方向/回退/快捷/媒体)。

## 验证
- 服务端握手/认证/ping、mDNS 广播 ✅
- Swift PBKDF2+HMAC 与 Go 逐字节一致 ✅
- Windows：go test 通过、smoke 客户端注入 move/text/key/快捷键无误、rmserver.exe 监听 ✅
- ⚠️ mac key 注入与 iOS 按键栏未在 mac 实测；Windows 媒体/中文长测待补

## 已知缺口
- 无 TLS / 数据面仍走 TCP（未上 UDP+AEAD）；移动事件无节流上限。
- iOS：仅 1234 默认密码、无记住设备、无重连；中文/特殊键未充分测。
- Windows SendInput 仅编译验证。

## 下一步（建议顺序）
1. Windows + mac 真机注入实测，修手感/节流。
2. 全输入：右键拖拽/双指/快捷键/媒体键。
3. UDP 数据面 + 加密、TLS、受信设备免密。
4. 锁屏（Win SYSTEM 服务）。

## 续接清单
README → proto/protocol-mvp.md → 本页 → 选下一里程碑。
