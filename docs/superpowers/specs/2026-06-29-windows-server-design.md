# Windows Server 实装 + 全输入 + 托盘自启 — 设计 (M3+部分M4)

> 状态：已定方向，待用户复审。目标：把仅编译验证的 Windows server 真机跑通，补全键盘/快捷键/媒体键，加轻量托盘+开机自启与构建脚本，并同步 iOS 客户端发送侧。

## 范围
- ✅ 修缮并真机验证 Windows `SendInput` 注入（鼠标/按键/滚动/Unicode 文本）。
- ✅ 全输入：方向键、回车/退格/Tab/Esc/Del、F1–F12、组合快捷键、媒体键。
- ✅ 托盘图标 + 注册表开机自启 + `build.ps1` 出 `rmserver.exe`。
- ✅ iOS 客户端加按键/快捷键/媒体键 UI 与发送（本机不构建，留 mac 编译）。
- ❌ 不做：TLS/UDP、锁屏 SYSTEM 服务、受信设备免密（后续里程碑）。

## 1. Keycode + mods 表（平台中立，写入 proto）
`key` 消息：`{t:"key", code:int, down:bool, mods:int}`。printable 文本仍走 `text`。

mods 位掩码：`ctrl=1, alt=2, shift=4, meta(win/cmd)=8`。

code 区间：
| 区间 | 含义 | 例 |
|------|------|----|
| 65–90 | 字母 A–Z（用于快捷键，配 mods） | Ctrl+C = code 67, mods 1 |
| 48–57 | 数字 0–9 | |
| 1 回车 2 退格 3 Tab 4 Esc 5 Del | 编辑键 | |
| 10 上 11 下 12 左 13 右 | 方向键 | |
| 20 Home 21 End 22 PgUp 23 PgDn | 导航 | |
| 30–41 | F1–F12 | |
| 50 音量− 51 音量+ 52 静音 53 播放/暂停 54 下一曲 55 上一曲 | 媒体 | |

服务端各平台把 code→VK(Windows)/CGKeyCode 或系统音量键(mac)。未知 code 忽略，向前兼容。

## 2. 协议改动 (`proto.go` + `protocol-mvp.md`)
- `In` 已有 `Code int`、`Mods int`；`Down` 复用于 key。
- `server.go` 增 `case "key": inj.Key(m.Code, m.Mods, down)`；文本仍 `text`。文档把 key 标为已实现并附 code 表。

## 3. 注入接口 (`injector.go`)
新增 `Key(code, mods int, down bool)`。各平台实现；stub 打日志。

## 4. Windows 注入 (`injector_windows.go`)
- 修正：64-bit INPUT 尺寸正确（type+union=40B），键盘事件不再以鼠标布局误覆盖；批量 `SendInput`。
- 鼠标移动/按键/中右键已具，校手感；滚动用 `WHEEL_DELTA(120)` 与方向。
- Unicode 文本 `KEYEVENTF_UNICODE` down/up 成对。
- Key：code→VK 映射 + mods（按下修饰→主键→抬起逆序）；媒体键用 VK_VOLUME_*/VK_MEDIA_*。

## 5. mac 对齐 (`injector_darwin.go`)
实现 `Key`：方向/编辑键映射常见 CGKeyCode，mods→CGEventFlags；媒体音量暂作日志桩，保证 mac 编译与现有功能不回归。

## 6. 托盘 + 自启 (`tray_windows.go`, windows-only)
- `getlantern/systray`：图标、菜单(显示密码/端口、开机自启开关、退出)。
- 注册表 `HKCU\...\Run` 写/删 `rmserver.exe -pass ...`。
- `-notray` 退回控制台。mac/其它仍控制台 `main`。

## 7. 构建 (`build.ps1`)
`go build -o rmserver.exe .` (GOOS=windows)；打印用法。

## 8. iOS (`ios/Sources`)
触控板视图加：方向键、回车/退格、Esc、常用快捷键(复制/粘贴/全选)、媒体键；发送 `key{code,down,mods}`。本机不编译，标注待 mac 验证。

## 9. 验证
- `go vet`/`go build` mac+win 双通。
- 真机：本机 `go run . -pass 1234`，连接后验鼠标/滚动/打字/快捷键/媒体键。
- 托盘出现、自启项写入/删除、`-notray` 控制台可跑。
- 更新 `docs/12-progress.md`。
