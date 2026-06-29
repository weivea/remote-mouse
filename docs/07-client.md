# 07 — Client 设计（iOS + Android）

## 框架：Flutter
一套码覆盖 iOS+Android，触控/手势好，发现/网络包齐全。

## 屏幕
1. 发现列表：mDNS 设备名/平台/IP，下拉刷新，手动加 IP。
2. 配对：输入 PC 密码/PIN，记住设备。
3. 触控板（主屏）：大触控区 + 左右键 + 滚动条；顶栏连接状态；底栏键盘/媒体/设置入口。
4. 键盘：唤起系统输入法发文本，特殊/快捷键浮层。
5. 媒体/演示：音量、播放、翻页。
6. 设置：灵敏度、反转、手势开关、保活。

## 触控映射
- 单指移动→相对位移（含加速曲线）；点按→左键；双指点→右键；双指滑→滚动；长按拖→拖拽；三指→可配。
- 高频采样合并，发当前最新位移；按设备刷新率节流。

## 网络
- 发现：`multicast_dns`/`bonjour_service`。
- 控制：`web_socket_channel` 或 TCP+TLS。
- 数据：`RawDatagramSocket` 发加密 UDP。
- 重连：断线指数退避，缓存上次设备免发现。

## 平台权限
- iOS：`NSLocalNetworkUsageDescription`、`NSBonjourServices`；后台保活有限，前台为主。
- Android：multicast lock + wifi 权限；前台服务可保活。

## 键盘/中文
- 隐藏 TextField 取系统 IME 输出，整段文本走控制面发 keyText，避免逐键丢；emoji/中文按 unicode 注入。

## 体验
- 触觉反馈、低延迟绘制、电量优化（停发时降频）。详见 [05](05-protocol.md)/[02](02-architecture.md)。
