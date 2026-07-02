import SwiftUI

// Air-mouse screen: the cursor is driven by tilting the phone (MotionController);
// this view hosts the buttons and the calibration flow. Layout per spec — left
// click on the left, a scroll strip in the middle, right click on the right, and
// a calibration switch in the bottom-left corner.
//
// Two modes (bottom-left switch):
//  - OFF: relative mode (default) — tilt/turn nudges the cursor.
//  - ON:  absolute/calibrated mode — after a short circle-wave calibration, the
//         phone's pointing maps to a fixed screen position.
//
// Grip assumption: held in portrait, top edge pointed forward like a remote.
struct AirMousePane: View {
    @EnvironmentObject var client: Client
    @StateObject private var motion = MotionController()
    @State private var calibrate = false

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
        .overlay { if motion.calibrating { calibrationOverlay } }
        .overlay { if !motion.available { unavailableOverlay } }
        .onAppear {
            motion.onMove = { dx, dy in client.move(dx, dy) }
            motion.onMoveAbs = { nx, ny in client.moveAbs(nx, ny) }
            client.requestScreen()
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

    @State private var scrollLast: CGFloat = 0

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
                    client.requestScreen() // refresh screen size for aspect
                    motion.beginCalibration()
                } else {
                    motion.cancelCalibration()
                    motion.setMode(.relative)
                }
            }

            Spacer()

            Label(statusText, systemImage: "gyroscope")
                .font(.caption).foregroundStyle(.secondary)
                .lineLimit(1).minimumScaleFactor(0.8)
        }
        .frame(height: 44)
    }

    private var statusText: String {
        if !motion.running { return "陀螺未启用" }
        return motion.mode == .absolute ? "绝对模式 · 指向移动光标" : "相对模式 · 倾斜移动光标"
    }

    // MARK: calibration overlay

    private var calibrationOverlay: some View {
        VStack(spacing: 16) {
            Image(systemName: "gyroscope").font(.system(size: 40))
                .symbolEffect(.pulse)
            Text("校准中").font(.title3.weight(.semibold))
            Text("保持指向屏幕，手腕带动手机缓慢转一圈\n划出你舒适的移动范围，然后点「完成」")
                .font(.footnote).foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            HStack(spacing: 12) {
                Button("取消") {
                    motion.cancelCalibration()
                    calibrate = false
                }
                .buttonStyle(.bordered)
                Button("完成") {
                    motion.finishCalibration(screenW: client.screenW, screenH: client.screenH)
                }
                .buttonStyle(.borderedProminent)
            }
        }
        .padding(28)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.ultraThinMaterial)
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
