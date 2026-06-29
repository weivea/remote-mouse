# Remote Mouse — 设计文档

把手机（iOS / Android）变成 PC（Windows / macOS）的无线触控板与键盘：在同一 WiFi 下自动发现电脑、密码认证后建立连接，发送鼠标/键盘信号控制光标。目标包括在锁屏状态下也能工作并辅助输入密码解锁。

> 本仓库当前阶段为 **调研 + 设计**。文档拆分为多个文件，方便分多次迭代推进，并让后续会话快速恢复上下文。代码尚未开始。

## 阅读顺序

| # | 文档 | 内容 |
|---|------|------|
| 00 | [overview](00-overview.md) | 愿景、范围、非目标、关键约束 |
| 01 | [requirements](01-requirements.md) | 功能/非功能需求、用户故事 |
| 02 | [architecture](02-architecture.md) | 总体架构、组件、数据流 |
| 03 | [tech-stack](03-tech-stack.md) | 技术选型与理由 |
| 04 | [discovery-pairing](04-discovery-pairing.md) | 设备发现、配对、密码认证 |
| 05 | [protocol](05-protocol.md) | 通信协议（消息/编码/握手）|
| 06 | [server](06-server.md) | Server（Win+Mac）设计与输入注入 |
| 07 | [client](07-client.md) | Client（iOS+Android）设计 |
| 08 | [security](08-security.md) | 加密、认证、密码处理 |
| 09 | [lock-screen](09-lock-screen.md) | 锁屏控制可行性（关键难点）|
| 10 | [roadmap](10-roadmap.md) | 里程碑与迭代计划 |
| 11 | [glossary](11-glossary.md) | 术语表 |

## 一句话结论

- **MVP 完全可行**：已登录会话内的鼠标/键盘控制，跨平台输入模拟库成熟（robotgo / enigo）。
- **Windows 锁屏可行**：需 SYSTEM 服务切到 input/secure desktop 注入，技术路径明确（AnyDesk/TeamViewer 即此原理）。
- **macOS 登录窗解锁基本不可行（纯软件）**：系统设计上禁止注入登录窗，需硬件 HID 桥才能保证；这是项目最大风险点。
- 传输用 **UDP（输入）+ TCP/WS（控制）**，发现用 **mDNS**，客户端用 **Flutter**。

## 当前状态

见 [roadmap](10-roadmap.md)。所有内容为设计草案，欢迎在迭代中修订。
