# iOS Client (SwiftUI)

原生 SwiftUI 触控板/键盘客户端。Bonjour 发现 + 密码认证 + 移动/点击/滚动/文本。协议见 [/proto/protocol-mvp.md](../proto/protocol-mvp.md)。

## 工程生成
工程用 [XcodeGen](https://github.com/yonsm/XcodeGen) 描述（`project.yml`），不入库 `.xcodeproj`：
```bash
brew install xcodegen
cd ios && xcodegen generate
open RemoteMouse.xcodeproj
```

## 构建（模拟器）
```bash
xcodebuild -project RemoteMouse.xcodeproj -scheme RemoteMouse \
  -sdk iphonesimulator -destination 'platform=iOS Simulator,name=iPhone 17 Pro' build
```

## 结构
- `Models/Crypto.swift`  PBKDF2-HMAC-SHA256 证明（与 Go 服务端一致）
- `Models/Discovery.swift`  NWBrowser 浏览 `_remotemouse._tcp`
- `Models/Client.swift`  NWConnection、握手认证、发送事件
- `Views/DiscoveryView.swift`  发现列表 + 密码 + 手动 IP
- `Views/ControlView.swift`  连接后容器：顶部悬浮按钮切换触控板 / 文本 / 全键盘，含断开；支持横竖屏
- `Views/TouchpadView.swift`  触控板界面：移动/轻点 + 滚动条 + 左右键
- `Views/TextInputView.swift`  文本界面：多行输入并发送到电脑
- `Views/KeyboardView.swift`  全键盘界面：常用键 / 编辑(复制粘贴) / 方向键 / 媒体键

> 真机/同 WiFi 测试连接电脑端 `server`。模拟器可用「手动连接 127.0.0.1:27500」。
