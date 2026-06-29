# 03 — 技术选型

## 总结
| 层 | 选型 | 理由 |
|----|------|------|
| Client | **iOS 原生 SwiftUI**（Android 后续） | 当前优先 iOS 手感与系统能力；Android 暂缓 |
| Server 核心 | **Go**（或 Rust 备选） | 跨平台、并发网络强、好打包，便于接平台原生注入 |
| 输入模拟 | **SendInput / CGEvent** | 当前已用平台原生 API 实现已登录会话注入 |
| 发现 | **mDNS/Bonjour** + UDP 广播兜底 | 零配置；广播应对多播被阻 |
| 控制面 | **TCP + TLS**（或 WSS） | 可靠、握手认证、易调试 |
| 数据面 | **UDP + 加密** | 低延迟、丢包容忍 |
| 锁屏 | 平台原生（Win 服务/C++；mac daemon/Swift） | 系统级权限，跨语言桥接 |

## Client：iOS SwiftUI
- 当前客户端位于 `ios/`，用 SwiftUI + Network.framework 实现。
- 发现：Bonjour / NWBrowser 浏览 `_remotemouse._tcp`。
- 网络：MVP 使用单 TCP + 换行 JSON；后续补 TLS 与 UDP 数据面。
- Android/跨平台客户端仍可作为后续扩展，不阻塞 iOS MVP。

## Server：Go vs Rust
- **Go(推荐)**：开发快、交叉编译、网络实现简单；当前直接接 Windows SendInput 与 macOS CGEvent。
- **Rust**：性能/安全好、enigo 干净，锁屏原生 FFI 顺；缺点开发慢。
建议 MVP 用 Go，锁屏特权模块用 C++/Swift，主程序 cgo/IPC 调。

## 锁屏原生
- Windows：C++ 服务，`SendInput`+`OpenInputDesktop`/`SetThreadDesktop`，可选 Credential Provider。
- macOS：Swift `LaunchDaemon`+CGEvent；登录窗受限（见 09）。

## 备选整包方案
- Electron+Node+`nut.js`：UI 易、包大、锁屏难、延迟一般，不推荐主线。
- 纯 Rust（Tauri UI + enigo）：体积小性能好，可作 Go 替代。

## 仓库结构（建议）
```
/ios      SwiftUI iOS client
/server   Go 主程序 + 平台 injector
/server/native/win  C++ 锁屏服务
/server/native/mac  Swift daemon
/proto    协议 schema
/docs
```
