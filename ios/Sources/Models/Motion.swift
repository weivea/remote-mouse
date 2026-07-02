import CoreMotion
import Foundation

// MotionController turns device attitude into cursor motion for the air-mouse
// screen. It reads CoreMotion `deviceMotion` in a gravity-referenced frame and
// projects the phone's "forward" (top-edge) vector into world space, yielding an
// azimuth (heading, horizontal) and elevation (tilt, vertical).
//
// Two modes:
//  - relative: per-frame azimuth/elevation *deltas* become relative move(dx,dy).
//    No calibration, robust, the default.
//  - absolute: azimuth/elevation *offsets* from a calibrated neutral map to an
//    absolute normalized screen position (0..65535). Calibration is a circle
//    wave that captures the comfortable neutral and horizontal range; the
//    vertical range follows the screen aspect so pointing stays isotropic.
//
// Why attitude and not the accelerometer: integrating linear acceleration for
// displacement drifts badly and jerks on sudden stops (a well-known dead end).
// Angular attitude is stable and is what real gyro mice use.
//
// Grip assumption: held in portrait, top edge pointed forward like a remote.
final class MotionController: ObservableObject {
    enum Mode { case relative, absolute }

    // relative deltas (px), delivered on the main thread.
    var onMove: ((Int, Int) -> Void)?
    // absolute normalized position 0..65535, delivered on the main thread.
    var onMoveAbs: ((Int, Int) -> Void)?

    @Published var available: Bool
    @Published var running: Bool = false
    @Published private(set) var mode: Mode = .relative
    @Published private(set) var calibrating: Bool = false

    // Relative tuning. Sensitivity is pixels per radian; dead zone drops
    // sub-tremor jitter; smoothing is the EMA factor on the output.
    private let sensitivity: Double = 1100
    private let deadZone: Double = 0.0009
    private let smoothing: Double = 0.35

    // Absolute tuning. Default half-ranges (used before/without calibration):
    // ±30° yaw, ±20° pitch. A minimum half-range guards against a too-small
    // calibration sweep making the cursor hair-trigger.
    private var halfRangeAz: Double = 30 * .pi / 180
    private var halfRangeEl: Double = 20 * .pi / 180
    private var neutralAz: Double = 0
    private var neutralEl: Double = 0
    private let minHalfRange: Double = 8 * .pi / 180
    private let absSmoothing: Double = 0.5

    private let mgr = CMMotionManager()
    private let queue = OperationQueue()

    // relative state
    private var hasPrev = false
    private var prevAz = 0.0
    private var prevEl = 0.0
    private var emaDX = 0.0
    private var emaDY = 0.0

    // absolute state
    private var absSeeded = false
    private var emaSX = 0.0
    private var emaSY = 0.0

    // calibration capture (circle wave) — offsets relative to a capture ref.
    private var capturing = false
    private var capSeeded = false
    private var capRefAz = 0.0
    private var capRefEl = 0.0
    private var minAzOff = 0.0, maxAzOff = 0.0
    private var minElOff = 0.0, maxElOff = 0.0

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
        resetRelative()
        absSeeded = false
        mgr.startDeviceMotionUpdates(using: bestFrame(), to: queue) { [weak self] m, _ in
            guard let self, let m else { return }
            self.handle(m)
        }
        DispatchQueue.main.async { self.running = true }
    }

    func stop() {
        if mgr.isDeviceMotionActive { mgr.stopDeviceMotionUpdates() }
        resetRelative()
        DispatchQueue.main.async { self.running = false }
    }

    // Freeze motion while the user touches a button/scroll so hand movement from
    // the tap doesn't disturb the cursor. Relative resets its baseline on resume
    // so no accumulated jump is applied; absolute simply resumes.
    func setPaused(_ p: Bool) {
        queue.addOperation { [weak self] in
            guard let self else { return }
            self.paused = p
            if !p { self.hasPrev = false }
        }
    }

    func setMode(_ m: Mode) {
        queue.addOperation { [weak self] in
            guard let self else { return }
            self.hasPrev = false
            self.absSeeded = false
            DispatchQueue.main.async { self.mode = m }
        }
    }

    // Begin capturing a circle wave. Cursor is held (no output) until finished.
    func beginCalibration() {
        queue.addOperation { [weak self] in
            guard let self else { return }
            self.capturing = true
            self.capSeeded = false
            self.minAzOff = 0; self.maxAzOff = 0
            self.minElOff = 0; self.maxElOff = 0
            DispatchQueue.main.async { self.calibrating = true }
        }
    }

    // Finish capture: derive neutral (center of the sweep) and half-ranges, then
    // switch to absolute mode. Vertical range is set from the horizontal range
    // and the screen aspect so pointing is isotropic; if the screen size is
    // unknown (0), the captured vertical sweep is used instead.
    func finishCalibration(screenW: Int, screenH: Int) {
        queue.addOperation { [weak self] in
            guard let self else { return }
            self.capturing = false
            let haz = max((self.maxAzOff - self.minAzOff) / 2, self.minHalfRange)
            self.neutralAz = normalizeAngle(self.capRefAz + (self.maxAzOff + self.minAzOff) / 2)
            self.neutralEl = self.capRefEl + (self.maxElOff + self.minElOff) / 2
            self.halfRangeAz = haz
            if screenW > 0 && screenH > 0 {
                self.halfRangeEl = max(haz * Double(screenH) / Double(screenW), self.minHalfRange)
            } else {
                self.halfRangeEl = max((self.maxElOff - self.minElOff) / 2, self.minHalfRange)
            }
            self.absSeeded = false
            DispatchQueue.main.async { self.calibrating = false; self.mode = .absolute }
        }
    }

    func cancelCalibration() {
        queue.addOperation { [weak self] in
            guard let self else { return }
            self.capturing = false
            DispatchQueue.main.async { self.calibrating = false }
        }
    }

    private func resetRelative() {
        hasPrev = false
        emaDX = 0; emaDY = 0
    }

    // Prefer the magnetometer-corrected frame (less yaw drift); fall back when a
    // device lacks a usable magnetometer.
    private func bestFrame() -> CMAttitudeReferenceFrame {
        let avail = CMMotionManager.availableAttitudeReferenceFrames()
        if avail.contains(.xArbitraryCorrectedZVertical) { return .xArbitraryCorrectedZVertical }
        return .xArbitraryZVertical
    }

    private func handle(_ m: CMDeviceMotion) {
        // World-space direction of the phone's top edge (device +Y). rotationMatrix
        // maps device axes into the reference frame, so column 2 is +Y in world.
        let r = m.attitude.rotationMatrix
        let fx = r.m12, fy = r.m22, fz = r.m32
        // Reference frame has Z vertical: azimuth is heading in the horizontal
        // plane, elevation is the tilt above it.
        let az = atan2(fy, fx)
        let el = atan2(fz, (fx * fx + fy * fy).squareRoot())

        if capturing { captureRange(az: az, el: el); return }
        if paused { prevAz = az; prevEl = el; return }

        switch mode {
        case .relative: emitRelative(az: az, el: el)
        case .absolute: emitAbsolute(az: az, el: el)
        }
    }

    private func captureRange(az: Double, el: Double) {
        if !capSeeded { capRefAz = az; capRefEl = el; capSeeded = true }
        var dAz = az - capRefAz
        if dAz > .pi { dAz -= 2 * .pi } else if dAz < -.pi { dAz += 2 * .pi }
        let dEl = el - capRefEl
        minAzOff = min(minAzOff, dAz); maxAzOff = max(maxAzOff, dAz)
        minElOff = min(minElOff, dEl); maxElOff = max(maxElOff, dEl)
    }

    private func emitRelative(az: Double, el: Double) {
        defer { prevAz = az; prevEl = el }
        guard hasPrev else { hasPrev = true; return }

        var dAz = az - prevAz
        if dAz > .pi { dAz -= 2 * .pi } else if dAz < -.pi { dAz += 2 * .pi }
        let dEl = el - prevEl

        let mAz = abs(dAz) < deadZone ? 0 : dAz
        let mEl = abs(dEl) < deadZone ? 0 : dEl

        // Sweeping right raises azimuth -> cursor right; tilting down raises
        // elevation -> cursor down (screen y grows downward).
        let rawDX = mAz * sensitivity
        let rawDY = mEl * sensitivity
        emaDX = smoothing * rawDX + (1 - smoothing) * emaDX
        emaDY = smoothing * rawDY + (1 - smoothing) * emaDY

        let dx = Int(emaDX.rounded())
        let dy = Int(emaDY.rounded())
        guard dx != 0 || dy != 0 else { return }
        DispatchQueue.main.async { self.onMove?(dx, dy) }
    }

    private func emitAbsolute(az: Double, el: Double) {
        var dAz = az - neutralAz
        if dAz > .pi { dAz -= 2 * .pi } else if dAz < -.pi { dAz += 2 * .pi }
        let dEl = el - neutralEl

        let fx = clamp(dAz / halfRangeAz, -1, 1)
        let fy = clamp(dEl / halfRangeEl, -1, 1)
        let sx = 0.5 + 0.5 * fx // az above neutral -> right
        let sy = 0.5 + 0.5 * fy // el above neutral -> down

        if !absSeeded { emaSX = sx; emaSY = sy; absSeeded = true }
        emaSX = absSmoothing * sx + (1 - absSmoothing) * emaSX
        emaSY = absSmoothing * sy + (1 - absSmoothing) * emaSY

        let nx = Int((clamp(emaSX, 0, 1) * 65535).rounded())
        let ny = Int((clamp(emaSY, 0, 1) * 65535).rounded())
        DispatchQueue.main.async { self.onMoveAbs?(nx, ny) }
    }
}

private func normalizeAngle(_ a: Double) -> Double {
    var x = a
    while x > .pi { x -= 2 * .pi }
    while x < -.pi { x += 2 * .pi }
    return x
}

private func clamp(_ v: Double, _ lo: Double, _ hi: Double) -> Double {
    min(max(v, lo), hi)
}
