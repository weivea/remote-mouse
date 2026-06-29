# 07 — Client 设计（当前 iOS，Android 后续）

## 框架：SwiftUI
当前已改为 iOS 原生 SwiftUI 客户端，工程在 `ios/`，用 XcodeGen 生成 `.xcodeproj`。Android / 跨平台客户端作为后续扩展。

## 屏幕
1. 发现列表：Bonjour 设备名/端点，支持手动 IP:port。
2. 配对：输入 PC 密码/PIN；记住设备待补。
3. 触控板（主屏）：大触控区 + 左右键 + 右侧滚动条 + 文本输入框。
4. 按键栏：方向键、回车/退格/Esc、复制/粘贴/全选、音量/播放。
5. 设置：灵敏度、反转、手势开关、保活待补。

## 触控映射
- 单指移动→相对位移；点按→左键；右侧滚动条→滚动。
- 双指点/双指滑、长按拖拽、移动事件节流与灵敏度配置待补。

## 网络
- 发现：Network.framework `NWBrowser` 浏览 `_remotemouse._tcp`。
- 控制：MVP 使用 TCP + 换行 JSON。
- 数据：暂未拆 UDP；后续用会话密钥加密 UDP。
- 重连：断线指数退避，缓存上次设备免发现。

## 平台权限
- iOS：`NSLocalNetworkUsageDescription`、`NSBonjourServices`；后台保活有限，前台为主。
- Android：multicast lock + wifi 权限；前台服务可保活。

## 键盘/中文
- 隐藏 TextField 取系统 IME 输出，整段文本走控制面发 keyText，避免逐键丢；emoji/中文按 unicode 注入。

## 体验
- 触觉反馈、低延迟绘制、电量优化（停发时降频）。详见 [05](05-protocol.md)/[02](02-architecture.md)。
