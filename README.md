# remote-mouse

把手机变成 PC 的无线触控板 + 键盘。Server 装在电脑（Windows/macOS），Client 是手机 App（iOS），同一 WiFi 下发现、密码认证后控制光标与键盘。

> M1/M2 已落地：iOS(SwiftUI) ↔ Go 服务端，mDNS 发现 + 密码认证 + 鼠标/滚动/文本，mac 端到端可跑通；Windows 仅换注入实现。设计文档见 [`docs/`](docs/README.md)。

## 目录
- [proto/](proto/protocol-mvp.md) — MVP 通信协议
- [server/](server/README.md) — Go 服务端（mac/Windows）
- [ios/](ios/README.md) — SwiftUI 客户端

## 快速了解
- 项目概述：[docs/00-overview.md](docs/00-overview.md)
- 协议：[proto/protocol-mvp.md](proto/protocol-mvp.md)
- 锁屏难点（关键）：[docs/09-lock-screen.md](docs/09-lock-screen.md)
- 迭代路线：[docs/10-roadmap.md](docs/10-roadmap.md)

## License
MIT