# Server (Go)

跨平台控制服务端：mDNS 广播 + TCP 控制面 + 密码认证 + 输入注入。
注入实现按平台分文件：`injector_darwin.go`(CGEvent)、`injector_windows.go`(SendInput)、`injector_other.go`(日志桩)。协议见 [/proto/protocol-mvp.md](../proto/protocol-mvp.md)。

## 运行（macOS 测试）
```bash
go run .            # 默认 pass=1234 port=27500，名取主机名
go run . -pass 6666 -name 客厅PC -port 27500
```
macOS 首次注入需在「系统设置→隐私与安全→辅助功能」授权运行进程。

## 构建 Windows
```powershell
cd server; .\build.ps1        # 出 rmserver.exe（窗口 + 托盘）
```
`build.ps1` 首次用 `github.com/akavel/rsrc` 把 `assets/manifest.xml`（Common Controls v6 + DPI 感知）与 `assets/tray.ico` 生成 `rsrc.syso`，再 `go build -ldflags "-H windowsgui" -o rmserver.exe .`。需 Go ≥ 1.21。

## 运行 Windows
```powershell
.\rmserver.exe -pass 1234             # 打开「控制面板」窗口 + 托盘（见下节，推荐）
.\rmserver.exe -pass 1234 -standalone # 旧版简易窗口（只读连接信息 + 设备列表）
.\rmserver.exe -pass 1234 -notray     # 纯后台，无窗口无托盘
```
默认打开集成「控制面板」（见下节）。`-standalone` 为旧版简易窗口：展示推荐局域网 IP（高亮 + 一键复制）、端口、密码、其余地址，以及实时在线设备列表（设备名/IP/时长/状态）。两者关闭窗口（×）均隐藏到托盘不退出；托盘左键单击切换窗口显隐，右键菜单含 **显示窗口 / 登录时打开界面 / 退出**。
SendInput 注入，无需额外授权（已登录会话）。支持鼠标/滚动/Unicode 文本/方向键/快捷键/媒体键。

## 控制面板（推荐）

双击或 `.\rmserver.exe -pass 1234 -port 27500` 打开「RemoteMouse 控制面板」单窗口即可完成一切：

- **连接信息**：推荐 IP（只读+复制）、端口/密码/设备名**可编辑**，「保存并应用」自动生效（服务在运行则重启服务，约 1 秒；独立运行则进程内热重配，已连接手机不掉线）。
- **服务管理**：实时状态；「启动类型」下拉（自动/手动/禁用，即“服务开机自启”）；「安装并启动 / 启动 / 停止 / 卸载」按钮按状态启停用。管理操作会弹 UAC 请求管理员。
- **已连接设备**：独立模式实时显示；服务模式经状态管道显示（约 1 秒刷新）。

命令行等价物（供脚本/排障，均需管理员）：

    .\rmserver.exe -install-service -pass 1234 -port 27500
    .\rmserver.exe -uninstall-service
    .\rmserver.exe -start-service
    .\rmserver.exe -stop-service
    .\rmserver.exe -set-start-type auto|manual|disabled
    .\rmserver.exe -apply-config -pass 1234 -port 27500 -name 客厅PC

## 安装（登录时打开界面）
1. 把 `rmserver.exe` 放到固定目录（如 `C:\Tools\RemoteMouse\`）。
2. 双击运行，托盘右键菜单勾选 **登录时打开界面** —— 写入 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 值 `RemoteMouse`（即 `"exe路径" -pass 密码 -port 端口 -name 设备名`）。
3. 下次登录自动打开控制面板。

> 区分两个「自启」概念：托盘的 **登录时打开界面** 是当前用户登录后打开本程序界面（HKCU Run 项）；控制面板里「启动类型 = 自动」是让 **RemoteMouse 服务** 随机器启动（开机即在，无需登录）。二者独立。

## 删除（卸载）
1. 托盘右键取消勾选 **登录时打开界面**，再点 **退出**；或手动删自启项：
   ```powershell
   Remove-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name RemoteMouse -ErrorAction SilentlyContinue
   ```
2. 删除 exe 与目录即可（无注册表残留）。

## 锁屏注入（Win SYSTEM 服务，M5 POC）
锁屏/登录界面运行在受保护的安全桌面，普通进程注入被系统禁止。方案：把 `rmserver.exe` 装成 **LocalSystem 服务**，服务持 TCP + 鉴权 + 事件解析，经命名管道把事件转发给按当前输入桌面（`Default`/`Winlogon`…）动态重开的 `-agent` 进程，由 agent 在该桌面 `SendInput`；锁屏时切到 winlogon token 的安全桌面 agent，从 iPhone 输入 Hello PIN 解锁。

```powershell
.\rmserver.exe -install-service -pass 1234 -port 27500   # 安装并自动启动（管理员）
.\rmserver.exe -uninstall-service                         # 停止并卸载（管理员）
```
日志：`C:\ProgramData\RemoteMouse\{service,agent}.log`（不记录 PIN）。完整**真机验证清单**见 [docs/09-lock-screen.md](../docs/09-lock-screen.md)。仍是 POC：PIN 走明文 LAN，仅限可信网络，TLS 待补。

## 现状
mac 端到端跑通；Windows 真机注入（鼠标/滚动/文本/键盘/快捷键）已验证。锁屏注入（Win SYSTEM 服务，M5 POC）代码已完成，待真机端到端验证。
