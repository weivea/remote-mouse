import SwiftUI

// Touchpad screen: drag to move / tap to click, a scroll strip, and click buttons.
// Layout adapts to orientation — buttons sit below in portrait and flank the pad
// in landscape (compact height) for two-thumb reach.
struct TouchpadPane: View {
    @EnvironmentObject var client: Client
    @Environment(\.verticalSizeClass) private var vSize
    @State private var last: CGSize = .zero
    @State private var scrollLast: CGFloat = 0
    private let sens: CGFloat = 1.6

    private var landscape: Bool { vSize == .compact }

    var body: some View {
        if landscape {
            HStack(spacing: 10) {
                clickButton("左键", left: true).frame(width: 92)
                touchpad
                scrollStrip.frame(width: 50)
                clickButton("右键", left: false).frame(width: 92)
            }
        } else {
            VStack(spacing: 12) {
                HStack(spacing: 10) {
                    touchpad
                    scrollStrip.frame(width: 50)
                }
                HStack(spacing: 10) {
                    clickButton("左键", left: true)
                    clickButton("右键", left: false)
                }
                .frame(height: 64)
            }
        }
    }

    @ViewBuilder private func clickButton(_ title: String, left: Bool) -> some View {
        if left {
            Button { client.click("left") } label: { buttonLabel(title) }
                .buttonStyle(.borderedProminent)
        } else {
            Button { client.click("right") } label: { buttonLabel(title) }
                .buttonStyle(.bordered)
        }
    }

    private func buttonLabel(_ title: String) -> some View {
        Text(title).font(.headline)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var touchpad: some View {
        RoundedRectangle(cornerRadius: 16).fill(Color(.secondarySystemBackground))
            .overlay(
                VStack(spacing: 6) {
                    Image(systemName: "hand.point.up.left").font(.title2)
                    Text("拖动移动光标 · 轻点左键").font(.footnote)
                }.foregroundStyle(.secondary)
            )
            .contentShape(Rectangle())
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
        RoundedRectangle(cornerRadius: 16).fill(Color(.secondarySystemBackground))
            .overlay(
                VStack(spacing: 6) {
                    Image(systemName: "chevron.up")
                    Image(systemName: "arrow.up.arrow.down")
                    Image(systemName: "chevron.down")
                }.font(.footnote).foregroundStyle(.secondary)
            )
            .contentShape(Rectangle())
            .gesture(DragGesture(minimumDistance: 0)
                .onChanged { v in
                    let dy = v.translation.height - scrollLast
                    if abs(dy) >= 2 { client.scroll(0, Int(-dy)); scrollLast = v.translation.height }
                }
                .onEnded { _ in scrollLast = 0 })
    }
}
