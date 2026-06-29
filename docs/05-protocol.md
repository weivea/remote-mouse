# 05 — 通信协议

## 双通道
| 通道 | 传输 | 用途 | 特性 |
|------|------|------|------|
| 控制面 | TCP+TLS / WSS | 握手、认证、配置、心跳 | 可靠有序 |
| 数据面 | UDP（会话密钥加密） | 鼠标/键盘高频事件 | 低延迟、可丢、最新优先 |

端口建议：发现 5353(mDNS)/自定义广播；控制 TCP 27500；数据 UDP 27501（可在 TXT/握手协商）。

## 编码
- 控制面：JSON（易调试）或 protobuf。MVP 用 JSON。
- 数据面：紧凑二进制（定长头 + 小载荷），省带宽降延迟。

## 控制消息（JSON）
```
HELLO    {devid,name,platform,ver}
PAIR_REQ {}              // 服务端要求配对
PROOF    {hmac/spake}    // 认证证明
AUTH_OK  {sessionKey,server}
CONFIG   {sensitivity,...}
PING/PONG{ts}
BYE      {}
ERROR    {code,msg}
```

## 数据面二进制帧
```
[1] ver|type  [1] flags  [2] seq  [8] payload...
type: 1 moveRel 2 button 3 scroll 4 keyText 5 keyTap 6 media
moveRel: int16 dx, int16 dy
button : u8 btn, u8 down
scroll : int16 dx, int16 dy
keyTap : u16 code, u8 mods
keyText: utf8 (走控制面避免丢)
```
- seq 单调递增，过期丢弃；移动可合并最新。
- 文本/快捷键这类不可丢的走控制面 TCP。

## 握手
1. TCP+TLS → HELLO → 首次 PAIR_REQ；2. SPAKE2/HMAC 认证 → AUTH_OK 带 sessionKey；3. UDP 帧用 sessionKey AEAD 加密、随机 nonce+seq 防重放；4. 心跳 PING/PONG，超时断开。

## 版本/兼容
- 头含 ver；HELLO 协商；未知 type 忽略。
- 未来加：手势包、剪贴板、文件（走控制面）。详见 [08-security](08-security.md)。
