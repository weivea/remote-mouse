# 12 — 当前进度

> 单一事实来源：每次迭代结束更新此页，新会话先读这里。里程碑定义见 [10-roadmap](10-roadmap.md)。

**更新时间**：2026-06-29 ｜ **状态**：M1+M2 完成（mac 端到端跑通）

## 已实现
- **协议** `proto/protocol-mvp.md`：单 TCP + 换行 JSON；PBKDF2-HMAC 挑战-响应认证；move/click/button/scroll/text/ping。
- **Server** `server/`（Go）：mDNS 广播 + TCP 监听 + 认证 + 注入抽象。mac=CGEvent、Windows=SendInput、其它=日志桩。mac/Win 均编译通过。
- **iOS** `ios/`（SwiftUI，xcodegen）：Bonjour 发现、密码/手动 IP、触控板（移动/左右键/滚动/文本）。模拟器构建+运行通过。

## 验证
- 服务端握手/认证/ping、mDNS 广播 ✅
- Swift PBKDF2+HMAC 与 Go 逐字节一致 ✅
- iOS 构建成功、模拟器启动渲染正常 ✅
- ⚠️ 真机同 WiFi、mac 注入（需辅助功能授权）、Windows 注入实机：未实测

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
