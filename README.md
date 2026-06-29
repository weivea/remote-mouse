# remote-mouse

把手机变成 PC 的无线触控板 + 键盘。Server 装在电脑（Windows/macOS），Client 是手机 App（iOS），同一 WiFi 下发现、密码认证后控制光标与键盘。

> M1/M2 已落地：iOS(SwiftUI) ↔ Go 服务端，mDNS 发现 + 密码认证 + 鼠标/滚动/文本，mac 端到端可跑通；Windows 仅换注入实现。设计文档见 [`docs/`](docs/README.md)。

## 目录
- [proto/](proto/protocol-mvp.md) — MVP 通信协议
- [server/](server/README.md) — Go 服务端（mac/Windows）
- [ios/](ios/README.md) — SwiftUI 客户端

## 启动指南

> 同一 WiFi 下：先开电脑端 server，再在手机/模拟器开 client，输入密码连接。

### 1) 启动 Server（电脑端）
依赖：[Go](https://go.dev) ≥ 1.21。
```bash
cd server
go run . -pass 1234            # 默认 port=27500，名取主机名
go run . -pass 6666 -name 客厅PC -port 27500   # 自定义
```
- macOS 首次运行需在「系统设置 → 隐私与安全 → 辅助功能」勾选运行进程（终端/IDE），否则无法移动光标。
- Windows：`GOOS=windows GOARCH=amd64 go build -o rmserver.exe .` 后运行 `rmserver.exe -pass 1234`，已登录会话免授权。
- 启动后日志会打印 `password=...`、监听端口与 mDNS 名称。

### 2) 启动 iOS Client
依赖：Xcode、`brew install xcodegen`。
```bash
cd ios
xcodegen generate
open RemoteMouse.xcodeproj   # 选目标设备/模拟器，Run
```
- **真机（同 WiFi）**：「发现的电脑」会自动列出 server，点选 → 输入密码 → 连接。
- **模拟器**：用「手动连接」填 `127.0.0.1:27500`，输入密码连接。
- 连上后即触控板：拖动移光标、轻点左键、右侧条滚动、文本框回车发送文字。

命令行直接装到模拟器：
```bash
xcodebuild -project ios/RemoteMouse.xcodeproj -scheme RemoteMouse \
  -sdk iphonesimulator -destination 'platform=iOS Simulator,name=iPhone 17 Pro' build
```

## 快速了解
- 当前进度：[docs/12-progress.md](docs/12-progress.md)
- 项目概述：[docs/00-overview.md](docs/00-overview.md)
- 协议：[proto/protocol-mvp.md](proto/protocol-mvp.md)
- 锁屏难点（关键）：[docs/09-lock-screen.md](docs/09-lock-screen.md)
- 迭代路线：[docs/10-roadmap.md](docs/10-roadmap.md)

## License
MIT