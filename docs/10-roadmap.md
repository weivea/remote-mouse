# 10 — 迭代路线

分多次会话推进，每阶段可独立验收。

## M0 — 设计 ✅
- 调研 + docs/。

## M1 — MVP 单平台直连 ✅
- Server(Go)：TCP 监听 + 注入（mac CGEvent / win SendInput）。
- Client(原生 SwiftUI)：手动输 IP，触控板移动+左右键+滚动+文本。
- 现状：mac 端到端跑通。Client 由 Flutter 改为 iOS 原生 SwiftUI。

## M2 — 发现 + 认证 ✅(mDNS+密码)
- mDNS announce/浏览；密码 HMAC(PBKDF2) 挑战-响应。TLS 待补。
- 验收：自动发现、密码连、错误被拒。

## M3 — 全输入 + 双平台 Server ◐
- 已完成：Windows SendInput 真机注入、键盘/快捷键/媒体键映射、Windows 集成控制面板（装/卸/启停服务、启动类型、热改端口/密码/设备名、端口仲裁、服务模式设备列表）与开机自启；iOS 按键栏。
- 待完成：mac key/媒体真机验证，双指/拖拽/移动节流，UDP 数据面 + 加密。验收：Win+Mac 输入稳定、延迟达标、稳定打字。

## M4 — 体验 + 受信设备
- 灵敏度/手势/记住设备/重连；Windows 托盘自启已提前完成。
- 验收：免密复连、配置生效。

## M5 — 锁屏（拉伸）
- Win SYSTEM 服务 secure-desktop 注入；mac 现状/硬件桥评估。
- 验收：Win 锁屏控制+解锁。

## M6 — 打磨发布
- 签名/公证/安装包，多设备测试。

## 后续可选
剪贴板、文件、多 server、横竖屏、绝对触控、Linux server、桌面 client。

## 续接清单（新会话先读）
1. README→09→10；2. 看 plan.md；3. 选下个里程碑落地。
