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
.\rmserver.exe -pass 1234     # 托盘图标：显示密码/端口、开机自启、退出
.\rmserver.exe -pass 1234 -notray   # 控制台
```
Windows 用 SendInput 注入，无需额外授权（已登录会话）。支持鼠标/滚动/Unicode 文本/方向键/快捷键/媒体键。开机自启写 HKCU\...\Run。

## 现状
mac 端到端可跑通：发现→认证→鼠标/滚动/文本。锁屏注入（Win SYSTEM 服务）属后续里程碑。
