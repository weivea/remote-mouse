import SwiftUI

// Full-keyboard screen. Each row is an HStack of equal-width keys, so keys grow
// and shrink with the available width and stay balanced in both orientations.
struct KeyboardPane: View {
    @EnvironmentObject var client: Client

    var body: some View {
        GeometryReader { geo in
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                section("常用") {
                    HStack(spacing: 8) {
                        iconKey("escape", "Esc") { client.key(K.esc) }
                        iconKey("arrow.right.to.line", "Tab") { client.key(K.tab) }
                        iconKey("delete.left", "退格") { client.key(K.back) }
                        iconKey("arrow.turn.down.left", "回车") { client.key(K.enter) }
                    }
                }
                section("编辑") {
                    HStack(spacing: 8) {
                        textKey("复制") { client.key(67, mods: K.ctrl) }
                        textKey("剪切") { client.key(88, mods: K.ctrl) }
                        textKey("粘贴") { client.key(86, mods: K.ctrl) }
                        textKey("全选") { client.key(65, mods: K.ctrl) }
                    }
                }
                section("方向键") { arrowCluster }
                section("媒体") {
                    VStack(spacing: 8) {
                        HStack(spacing: 8) {
                            iconKey("backward.fill", "上一曲") { client.key(K.prev) }
                            iconKey("playpause.fill", "播放") { client.key(K.playPause) }
                            iconKey("forward.fill", "下一曲") { client.key(K.next) }
                        }
                        HStack(spacing: 8) {
                            iconKey("speaker.minus.fill", "音量−") { client.key(K.volDown) }
                            iconKey("speaker.slash.fill", "静音") { client.key(K.mute) }
                            iconKey("speaker.plus.fill", "音量+") { client.key(K.volUp) }
                        }
                    }
                }
            }
            .frame(width: geo.size.width, alignment: .leading)
            .padding(.bottom, 8)
        }
        .scrollBounceBehavior(.basedOnSize)
        }
    }

    // Up on top, left/down/right below — centered so it reads like a real d-pad.
    private var arrowCluster: some View {
        VStack(spacing: 8) {
            arrowKey("arrow.up", K.up)
            HStack(spacing: 8) {
                arrowKey("arrow.left", K.left)
                arrowKey("arrow.down", K.down)
                arrowKey("arrow.right", K.right)
            }
        }
        .frame(maxWidth: .infinity)
    }

    @ViewBuilder private func section(_ title: String, @ViewBuilder _ content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(title).font(.footnote.weight(.semibold)).foregroundStyle(.secondary).padding(.leading, 4)
            content()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func iconKey(_ icon: String, _ label: String?, _ action: @escaping () -> Void) -> some View {
        Button(action: action) {
            VStack(spacing: 3) {
                Image(systemName: icon).font(.title3)
                if let label { Text(label).font(.caption2) }
            }
            .frame(maxWidth: .infinity).frame(height: 54)
        }
        .buttonStyle(.bordered)
    }

    private func textKey(_ label: String, _ action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(label).font(.subheadline.weight(.medium))
                .frame(maxWidth: .infinity).frame(height: 54)
        }
        .buttonStyle(.bordered)
    }

    private func arrowKey(_ icon: String, _ code: Int) -> some View {
        Button { client.key(code) } label: {
            Image(systemName: icon).font(.title3).frame(width: 92, height: 54)
        }
        .buttonStyle(.bordered)
    }
}
