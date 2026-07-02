import SwiftUI

// The three post-connection screens, switched from the floating top bar.
enum ControlTab: CaseIterable, Identifiable {
    case touchpad, airmouse, text, keyboard
    var id: Self { self }

    var title: String {
        switch self {
        case .touchpad: return "触控板"
        case .airmouse: return "飞鼠"
        case .text: return "文本"
        case .keyboard: return "键盘"
        }
    }

    var icon: String {
        switch self {
        case .touchpad: return "rectangle.and.hand.point.up.left.fill"
        case .airmouse: return "gyroscope"
        case .text: return "text.cursor"
        case .keyboard: return "keyboard"
        }
    }
}

// Connected root: one pane at a time under a floating top bar. The bar lives in
// the top safe-area inset so it never overlaps content and adapts to any screen
// size or orientation without magic offsets.
struct ControlView: View {
    @EnvironmentObject var client: Client
    @State private var tab: ControlTab = .touchpad

    var body: some View {
        pane
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .padding(.horizontal)
            .padding(.bottom, 10)
            .background(Color(.systemBackground))
            .safeAreaInset(edge: .top, spacing: 0) {
                topBar
                    .padding(.horizontal)
                    .padding(.top, 6)
                    .padding(.bottom, 8)
                    .background(Color(.systemBackground))
            }
    }

    @ViewBuilder private var pane: some View {
        switch tab {
        case .touchpad: TouchpadPane()
        case .airmouse: AirMousePane()
        case .text: TextInputPane()
        case .keyboard: KeyboardPane()
        }
    }

    private var topBar: some View {
        HStack(spacing: 10) {
            FloatingTabBar(tab: $tab)
            disconnectButton
        }
    }

    private var disconnectButton: some View {
        Button { client.disconnect() } label: {
            Image(systemName: "power")
                .font(.system(size: 16, weight: .bold))
                .foregroundStyle(.white)
                .frame(width: 40, height: 40)
                .background(Color.red, in: Circle())
        }
        .accessibilityLabel("断开连接")
    }
}

struct FloatingTabBar: View {
    @Binding var tab: ControlTab

    var body: some View {
        HStack(spacing: 4) {
            ForEach(ControlTab.allCases) { t in
                Button {
                    withAnimation(.snappy(duration: 0.18)) { tab = t }
                } label: {
                    HStack(spacing: 6) {
                        Image(systemName: t.icon)
                        Text(t.title)
                    }
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
                    .padding(.vertical, 9)
                    .frame(maxWidth: .infinity)
                    .foregroundStyle(tab == t ? Color.white : Color.primary)
                    .background {
                        if tab == t { Capsule().fill(Color.accentColor) }
                    }
                    .contentShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
        .padding(4)
        .background(.ultraThinMaterial, in: Capsule())
        .frame(maxWidth: .infinity)
    }
}
