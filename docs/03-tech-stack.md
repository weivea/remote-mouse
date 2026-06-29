# 03 — 技术选型

## 总结
| 层 | 选型 | 理由 |
|----|------|------|
| Client | **Flutter** | 一套码覆盖 iOS+Android，触控/手势好，mDNS 包成熟 |
| Server 核心 | **Go**（或 Rust 备选） | 跨平台、并发网络强、robotgo 输入模拟、好打包 |
| 输入模拟 | **robotgo**(Go) / **enigo**(Rust) | 跨平台鼠标键盘注入（已登录会话） |
| 发现 | **mDNS/Bonjour** + UDP 广播兜底 | 零配置；广播应对多播被阻 |
| 控制面 | **TCP + TLS**（或 WSS） | 可靠、握手认证、易调试 |
| 数据面 | **UDP + 加密** | 低延迟、丢包容忍 |
| 锁屏 | 平台原生（Win 服务/C++；mac daemon/Swift） | 系统级权限，跨语言桥接 |

## Client：Flutter
- 触控板=GestureDetector 采相对位移；多指手势内置。
- 发现：`multicast_dns` 或 `bonjour_service`。
- 网络：`web_socket_channel`（控制）+ `RawDatagramSocket`（UDP）。
- 备选：React Native（生态熟）、原生（最优手感、双倍成本）。Flutter 折中最佳。

## Server：Go vs Rust
- **Go(推荐)**：开发快、交叉编译、robotgo 一站式（鼠标/键盘/屏幕）；缺点 CGo 依赖。
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
/client   Flutter
/server   Go 主程序 + 平台 injector
/server/native/win  C++ 锁屏服务
/server/native/mac  Swift daemon
/proto    协议 schema
/docs
```
