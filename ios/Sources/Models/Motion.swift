import CoreMotion
import Foundation

// MotionController turns device attitude into relative cursor deltas for the
// air-mouse screen (Phase 1). It reads CoreMotion `deviceMotion` in a
// gravity-referenced frame, projects the phone's "forward" (top-edge) vector
// into world space, and emits per-frame pixel deltas from the change in that
// vector's azimuth (horizontal) and elevation (vertical).
//
// Why attitude and not the accelerometer: integrating linear acceleration for
// displacement drifts badly and jerks on sudden stops (a well-known dead end
// for air mice). Angular attitude is stable and is what real gyro mice use.
//
// Phase 2 will add an absolute/calibrated mode (map yaw/pitch to an absolute
// screen position) behind the calibration switch; the hooks (pause/recenter)
// are already here.
final class MotionController: ObservableObject {
    // Emits relative pixel deltas (already scaled/smoothed) on the main thread.
    var onMove: ((Int, Int) -> Void)?

    @Published var available: Bool
    @Published var running: Bool = false

    // Tuning. Sensitivity is pixels per radian of angular change; ~1100 makes a
    // gentle wrist turn cross a laptop screen. Dead zone drops sub-tremor jitter;
    // smoothing is the EMA factor on the output (higher = snappier, noisier).
    private let sensitivity: Double = 1100
    private let deadZone: Double = 0.0009
    private let smoothing: Double = 0.35

    private let mgr = CMMotionManager()
    private let queue = OperationQueue()

    private var hasPrev = false
    private var prevAz = 0.0
    private var prevEl = 0.0
    private var emaDX = 0.0
    private var emaDY = 0.0
    private var paused = false

    init() {
        available = mgr.isDeviceMotionAvailable
        queue.name = "rm.motion"
        queue.maxConcurrentOperationCount = 1
    }

    func start() {
        guard mgr.isDeviceMotionAvailable else {
            DispatchQueue.main.async { self.available = false }
            return
        }
        guard !mgr.isDeviceMotionActive else { return }
        mgr.deviceMotionUpdateInterval = 1.0 / 60.0
        hasPrev = false
        emaDX = 0; emaDY = 0
        mgr.startDeviceMotionUpdates(using: bestFrame(), to: queue) { [weak self] m, _ in
            guard let self, let m else { return }
            self.handle(m)
        }
        DispatchQueue.main.async { self.running = true }
    }

    func stop() {
        if mgr.isDeviceMotionActive { mgr.stopDeviceMotionUpdates() }
        hasPrev = false
        DispatchQueue.main.async { self.running = false }
    }

    // Freeze motion while the user touches a button/scroll so hand movement from
    // the tap doesn't drag the cursor. On resume the baseline is dropped so the
    // attitude change accumulated while paused is not applied as one big jump.
    func setPaused(_ p: Bool) {
        paused = p
        if !p { hasPrev = false }
    }

    // Prefer the magnetometer-corrected frame (less yaw drift); fall back when a
    // device lacks a usable magnetometer.
    private func bestFrame() -> CMAttitudeReferenceFrame {
        let avail = CMMotionManager.availableAttitudeReferenceFrames()
        if avail.contains(.xArbitraryCorrectedZVertical) { return .xArbitraryCorrectedZVertical }
        return .xArbitraryZVertical
    }

    private func handle(_ m: CMDeviceMotion) {
        if paused { return }
        // World-space direction of the phone's top edge (device +Y). rotationMatrix
        // maps device axes into the reference frame, so column 2 is +Y in world.
        let r = m.attitude.rotationMatrix
        let fx = r.m12, fy = r.m22, fz = r.m32
        // Reference frame has Z vertical, so azimuth is heading in the horizontal
        // plane and elevation is the tilt above it.
        let az = atan2(fy, fx)
        let el = atan2(fz, (fx * fx + fy * fy).squareRoot())
        defer { prevAz = az; prevEl = el }
        guard hasPrev else { hasPrev = true; return }

        var dAz = az - prevAz
        if dAz > .pi { dAz -= 2 * .pi } else if dAz < -.pi { dAz += 2 * .pi }
        let dEl = el - prevEl

        let mAz = abs(dAz) < deadZone ? 0 : dAz
        let mEl = abs(dEl) < deadZone ? 0 : dEl

        // Sign chosen so sweeping the phone right moves the cursor right and
        // tilting up moves it up, for a portrait, top-forward grip.
        let rawDX = mAz * sensitivity
        let rawDY = mEl * sensitivity
        emaDX = smoothing * rawDX + (1 - smoothing) * emaDX
        emaDY = smoothing * rawDY + (1 - smoothing) * emaDY

        let dx = Int(emaDX.rounded())
        let dy = Int(emaDY.rounded())
        guard dx != 0 || dy != 0 else { return }
        DispatchQueue.main.async { self.onMove?(dx, dy) }
    }
}
