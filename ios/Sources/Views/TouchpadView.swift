import SwiftUI

struct TouchpadView: View {
    @EnvironmentObject var client: Client
    @State private var last: CGSize = .zero
    @State private var scrollLast: CGFloat = 0
    @State private var typed = ""
    private let sens: CGFloat = 1.6

    var body: some View {
        VStack(spacing: 10) {
            HStack {
                Text(client.serverName.isEmpty ? "已连接" : "已连接：\(client.serverName)")
                    .font(.subheadline).foregroundStyle(.secondary)
                Spacer()
                Button("断开") { client.disconnect() }.tint(.red)
            }.padding(.horizontal)

            HStack(spacing: 8) {
                touchpad
                scrollStrip.frame(width: 44)
            }
            HStack(spacing: 8) {
                Button("左键") { client.click("left") }.buttonStyle(.borderedProminent)
                Button("右键") { client.click("right") }.buttonStyle(.bordered)
            }.frame(height: 56).padding(.horizontal)

            TextField("输入文本回车发送", text: $typed)
                .textFieldStyle(.roundedBorder).padding(.horizontal)
                .onSubmit { if !typed.isEmpty { client.text(typed); typed = "" } }
            keyBar
            Spacer(minLength: 0)
        }
        .padding(.vertical)
    }

    private var keyBar: some View {
        VStack(spacing: 6) {
            HStack(spacing: 6) {
                key("delete.left", K.back)
                key("arrow.up", K.up)
                key("arrow.turn.down.left", K.enter)
                key("escape", K.esc)
            }
            HStack(spacing: 6) {
                key("arrow.left", K.left)
                key("arrow.down", K.down)
                key("arrow.right", K.right)
            }
            HStack(spacing: 6) {
                Button("复制") { client.key(67, mods: K.ctrl) }
                Button("粘贴") { client.key(86, mods: K.ctrl) }
                Button("全选") { client.key(65, mods: K.ctrl) }
            }.buttonStyle(.bordered).font(.footnote)
            HStack(spacing: 6) {
                key("speaker.minus", K.volDown)
                key("playpause", K.playPause)
                key("speaker.plus", K.volUp)
            }
        }.padding(.horizontal)
    }

    private func key(_ icon: String, _ code: Int) -> some View {
        Button { client.key(code) } label: {
            Image(systemName: icon).frame(maxWidth: .infinity).frame(height: 36)
        }.buttonStyle(.bordered)
    }

    private var touchpad: some View {
        RoundedRectangle(cornerRadius: 16).fill(Color(.secondarySystemBackground))
            .overlay(Text("触控板").foregroundStyle(.secondary))
            .gesture(DragGesture(minimumDistance: 0)
                .onChanged { v in
                    let dx = (v.translation.width - last.width) * sens
                    let dy = (v.translation.height - last.height) * sens
                    if abs(dx) >= 1 || abs(dy) >= 1 { client.move(Int(dx), Int(dy)); last = v.translation }
                }
                .onEnded { v in
                    if abs(v.translation.width) < 5 && abs(v.translation.height) < 5 { client.click("left") }
                    last = .zero
                })
    }

    private var scrollStrip: some View {
        RoundedRectangle(cornerRadius: 16).fill(Color(.tertiarySystemBackground))
            .overlay(Image(systemName: "arrow.up.arrow.down").foregroundStyle(.secondary))
            .gesture(DragGesture(minimumDistance: 0)
                .onChanged { v in
                    let dy = v.translation.height - scrollLast
                    if abs(dy) >= 2 { client.scroll(0, Int(-dy)); scrollLast = v.translation.height }
                }
                .onEnded { _ in scrollLast = 0 })
    }
}
