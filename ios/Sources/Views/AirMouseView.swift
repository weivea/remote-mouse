import SwiftUI

// Air-mouse screen (Phase 1, relative mode): the cursor is driven by tilting the
// phone (MotionController); this view only hosts the buttons. Layout per spec —
// left click on the left, a scroll strip in the middle, right click on the
// right, and a calibration switch in the bottom-left corner. The calibration
// switch is a placeholder here; Phase 2 wires it to an absolute/calibrated mode.
//
// Grip assumption: held in portrait, top edge pointed forward like a remote.
struct AirMousePane: View {
    @EnvironmentObject var client: Client
    @StateObject private var motion = MotionController()
    @State private var calibrate = false
    @State private var showCalibrateNote = false

    var body: some View {
        VStack(spacing: 12) {
            HStack(spacing: 10) {
                pressButton("左键", b: "left", prominent: true)
                scrollStrip.frame(width: 56)
                pressButton("右键", b: "right", prominent: false)
            }
            .frame(maxHeight: .infinity)

            bottomBar
        }
        .overlay { if !motion.available { unavailableOverlay } }
        .onAppear {
            motion.onMove = { dx, dy in client.move(dx, dy) }
            motion.start()
        }
        .onDisappear { motion.stop() }
    }

    // MARK: buttons

    // Press = button down, release = button up: a quick tap reads as a click and
    // a hold enables drag-select. Motion is paused for the whole press so the
    // hand movement of tapping doesn't slide the cursor off the target.
    private func pressButton(_ title: String, b: String, prominent: Bool) -> some View {
        PressButton(
            title: title,
            prominent: prominent,
            onDown: { motion.setPaused(true); client.button(b, true) },
            onUp: { client.button(b, false); motion.setPaused(false) }
        )
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
                    motion.setPaused(true)
                    let dy = v.translation.height - scrollLast
                    if abs(dy) >= 2 { client.scroll(0, Int(-dy)); scrollLast = v.translation.height }
                }
                .onEnded { _ in scrollLast = 0; motion.setPaused(false) })
    }

    @State private var scrollLast: CGFloat = 0

    // MARK: bottom bar

    private var bottomBar: some View {
        HStack {
            Toggle(isOn: $calibrate) {
                Label("校准", systemImage: "scope").font(.subheadline.weight(.medium))
            }
            .toggleStyle(.button)
            .buttonStyle(.bordered)
            .onChange(of: calibrate) { _, on in
                if on {
                    // Phase 2: enter absolute/calibrated mode here.
                    showCalibrateNote = true
                    calibrate = false
                }
            }

            Spacer()

            Label(motion.running ? "陀螺控制中 · 倾斜手机移动光标" : "陀螺未启用",
                  systemImage: "gyroscope")
                .font(.caption).foregroundStyle(.secondary)
                .lineLimit(1).minimumScaleFactor(0.8)
        }
        .frame(height: 44)
        .alert("绝对校准模式即将上线", isPresented: $showCalibrateNote) {
            Button("好", role: .cancel) {}
        } message: {
            Text("当前为相对模式：倾斜/转动手机即可移动光标。指向式绝对校准将在下一步开放。")
        }
    }

    private var unavailableOverlay: some View {
        VStack(spacing: 8) {
            Image(systemName: "gyroscope").font(.largeTitle)
            Text("此设备无陀螺仪").font(.headline)
            Text("飞行鼠标需在真机上使用（模拟器不支持）").font(.footnote)
                .multilineTextAlignment(.center)
        }
        .foregroundStyle(.secondary)
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.ultraThinMaterial)
    }
}

// A button that reports raw press-down / release instead of a single tap, so the
// caller can drive button-down/up (hold-to-drag) and pause motion for the press.
private struct PressButton: View {
    let title: String
    let prominent: Bool
    let onDown: () -> Void
    let onUp: () -> Void
    @State private var pressed = false

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: 14)
        Text(title)
            .font(.headline)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(fill, in: shape)
            .overlay { shape.strokeBorder(Color.accentColor.opacity(prominent ? 0 : 0.5)) }
            .foregroundStyle(prominent ? Color.white : Color.accentColor)
            .contentShape(shape)
            .gesture(DragGesture(minimumDistance: 0)
                .onChanged { _ in if !pressed { pressed = true; onDown() } }
                .onEnded { _ in if pressed { pressed = false; onUp() } })
    }

    private var fill: Color {
        if prominent { return Color.accentColor.opacity(pressed ? 0.7 : 1) }
        return Color.accentColor.opacity(pressed ? 0.2 : 0.08)
    }
}
