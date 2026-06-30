# 12 — 当前进度

> 单一事实来源：每次迭代结束更新此页，新会话先读这里。里程碑定义见 [10-roadmap](10-roadmap.md)。

**更新时间**：2026-06-30 ｜ **状态**：M1+M2 完成；M3 部分完成（Windows server 真机注入 + 键盘/快捷/媒体映射 + 可视化窗口/托盘 UI + 自启，iOS 按键栏）

## 已实现
- **协议** `proto/protocol-mvp.md`：单 TCP + 换行 JSON；PBKDF2-HMAC 挑战-响应认证；move/click/button/scroll/text/key/ping。key 含方向/编辑/F键/快捷键/媒体。
- **Server** `server/`（Go）：mDNS 广播 + TCP 监听 + 认证 + 注入抽象。mac=CGEvent、Windows=SendInput（已修尺寸/批量/键盘/媒体，本机真机跑通）、其它=日志桩。Windows 用 `lxn/walk` 原生窗口 + 托盘 UI：展示推荐 IP/端口/密码（一键复制）与实时在线设备列表（`ClientRegistry` 连接中枢 + `netinfo` IP 探测），注册表开机自启；`build.ps1`（嵌入清单+图标）出 rmserver.exe，`-notray` 纯后台。
- **iOS** `ios/`（SwiftUI，xcodegen）：Bonjour 发现、密码/手动 IP、触控板 + 按键栏(方向/回退/快捷/媒体)。

## 验证
- 服务端握手/认证/ping、mDNS 广播 ✅
- Swift PBKDF2+HMAC 与 Go 逐字节一致 ✅
- Windows：go test 通过、smoke 客户端注入 move/text/key/快捷键无误、rmserver.exe 监听 ✅
- ⚠️ mac key 注入与 iOS 按键栏未在 mac 实测；Windows 媒体键/中文长测待补

## 已知缺口
- 无 TLS / 数据面仍走 TCP（未上 UDP+AEAD）；移动事件无节流上限。
- iOS：仅 1234 默认密码、无记住设备、无重连；中文/特殊键未充分测。
- 输入体验：双指/拖拽/灵敏度配置未完成；mac 键盘与媒体键仍需真机验证。

## 下一步（建议顺序）
1. 收尾 M3：mac key 真机验证、Windows 媒体/中文长测、移动节流、双指/拖拽/灵敏度。
2. 安全与传输：UDP 数据面 + 加密、TLS、认证失败限速。
3. M4 体验：记住设备、重连、受信设备免密。
4. 锁屏（Win SYSTEM 服务）。

## 续接清单
README → proto/protocol-mvp.md → 本页 → 选下一里程碑。
