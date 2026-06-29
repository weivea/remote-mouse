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
cd server; .\build.ps1        # 出 rmserver.exe（托盘模式）
```
`build.ps1` = `go build -ldflags "-H windowsgui" -o rmserver.exe .`。需 Go ≥ 1.21。

## 运行 Windows
```powershell
.\rmserver.exe -pass 1234            # 托盘图标：显示密码/端口、开机自启、退出
.\rmserver.exe -pass 1234 -notray    # 无托盘控制台模式
```
SendInput 注入，无需额外授权（已登录会话）。支持鼠标/滚动/Unicode 文本/方向键/快捷键/媒体键。

## 安装（开机自启）
1. 把 `rmserver.exe` 放到固定目录（如 `C:\Tools\RemoteMouse\`）。
2. 双击运行，托盘菜单勾选 **Start at login** —— 写入 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 值 `RemoteMouse`（即 `"exe路径" -pass 密码 -port 端口`）。
3. 下次登录自动后台运行。

## 删除（卸载）
1. 托盘菜单取消勾选 **Start at login**，再点 **Quit**；或手动删自启项：
   ```powershell
   Remove-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name RemoteMouse -ErrorAction SilentlyContinue
   ```
2. 删除 exe 与目录即可（无注册表残留）。

## 现状
mac 端到端跑通；Windows 真机注入（鼠标/滚动/文本/键盘/快捷键）已验证。锁屏注入（Win SYSTEM 服务）属后续里程碑。
