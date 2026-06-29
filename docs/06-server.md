# 06 — Server 设计（Windows + macOS）

## 进程模型
- **前台 App**（托盘/菜单栏）：UI、设密码、发现 announce、监听连接、已登录会话注入。
- **特权服务**（可选，锁屏用）：Win=Windows Service(SYSTEM)，mac=LaunchDaemon(root)。前台↔服务用本地 socket/命名管道 IPC。
MVP 仅前台即可满足已登录控制；锁屏再加服务。

## 模块
1. 发现：mDNS announce + UDP 广播应答。
2. 监听：TCP/TLS 控制 + UDP 数据。
3. 认证会话：密码校验、密钥协商、受信设备表。
4. 注入引擎：平台抽象 `Injector`。
5. 锁屏代理：检测锁屏 → 切特权注入。
6. UI：设备名/端口/密码/灵敏度/自启/连接列表。

## 输入注入抽象
```
Injector{ moveRel,moveAbs,button,scroll,keyText,keyTap,media }
```
- 用 robotgo(Go) 或 enigo(Rust) 实现已登录会话。
- 相对移动取当前光标 + dx/dy；keyText 走 unicode 直接注入（中文/emoji）。

## Windows
- 注入：`SendInput`（Go 用 robotgo）。
- 锁屏：SYSTEM 服务 `OpenInputDesktop`/`SetThreadDesktop` 切 Winlogon/secure desktop 再 SendInput；解锁可配 Credential Provider。
- 自启：服务注册；UAC 安装期提权。

## macOS
- 注入：CGEvent（robotgo/enigo）；首启引导开 **辅助功能** 授权。
- 锁屏：登录窗注入被禁（见 [09](09-lock-screen.md)），daemon 也难；解锁列风险。
- 自启：LaunchAgent；签名/公证。

## 连接管理
- 1:1（MVP），后续多 client。心跳超时断开。断线保活待重连。

## 配置存储
- 本地：密码哈希、设备名/端口、受信设备、灵敏度。mac Keychain / Win DPAPI 存密钥。
- 详见 [09](09-lock-screen.md)/[08](08-security.md)。
