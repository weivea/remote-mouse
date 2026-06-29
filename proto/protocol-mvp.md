# Remote Mouse — MVP 协议规范 (v0.1)

iOS 客户端 ↔ Windows/macOS 服务端。范围：mDNS 发现 + 密码认证 + 鼠标/键盘事件。
本期 **单条 TCP + 换行分隔 JSON**；UDP/加密留待后续。

## 1. 发现 (mDNS / Bonjour)
- 服务类型：`_remotemouse._tcp.`（local 域）
- 端口：TCP 控制端口（默认 **27500**）
- TXT 记录：`name`（设备显示名）、`platform`（windows/macos）、`ver`、`devid`
- 客户端浏览该类型 → 解析 host:port → 列表展示。兜底：手动输 IP:port。

## 2. 传输
- 单条 TCP。每条消息一行：紧凑 JSON + `\n`。UTF-8。
- 通用字段 `t`=消息类型；其余按类型定。

## 3. 握手 + 认证（HMAC 挑战-响应）
密码不上线。`key = PBKDF2-HMAC-SHA256(pwd, salt, iter, 32B)`。

```
1) C→S  hello     {t,devid,name,platform,ver}
2) S→C  challenge {t,nonce(b64 16B),salt(b64 16B),iter}
3) C→S  auth      {t,proof}   proof=b64(HMAC_SHA256(key, nonce))
4) S→C  auth_ok   {t,server,ver}        // 成功
   或    error     {t,code,msg} 并断开   // 失败
```
- iter 默认 100000。失败限速：3 次错误后断开并短锁定。

## 4. 事件（认证后，C→S）
| t | 字段 | 含义 |
|------|------|------|
| move | dx,dy (int) | 相对移动，像素 |
| button | b("left"/"right"/"middle"), down(bool) | 按下/抬起 |
| click | b | 单击（按下+抬起，便捷） |
| scroll | dx,dy (int) | 滚动 |
| text | s (string) | 文本输入（含中文/emoji，unicode 注入）|
| key | code(int),mods(int) | 特殊键/快捷键（后续）|
| ping | ts(int ms) | 心跳；S→C 回 pong {ts} |

- 高频 move 客户端节流合并；服务端最新优先。
- 未知 t 忽略，保证前向兼容。版本在 hello/auth_ok 协商。

## 5. 端口/默认
- mDNS 5353；控制 TCP 27500。UDP 数据面 27501 预留（未启用）。

## 6. 后续
TLS、UDP 数据面 AEAD、SPAKE2、受信设备免密、手势/媒体键。见 docs/05,08。
