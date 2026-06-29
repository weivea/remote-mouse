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
            Spacer(minLength: 0)
        }
        .padding(.vertical)
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
