import Foundation
import Network

enum ClientState: Equatable {
    case idle, connecting, authenticating, connected, failed(String)
}

final class Client: ObservableObject {
    @Published var state: ClientState = .idle
    @Published var serverName: String = ""
    // Primary-screen size reported by the server via getscreen; 0 until known.
    @Published var screenW: Int = 0
    @Published var screenH: Int = 0

    private var conn: NWConnection?
    private var password = ""
    private var buf = Data()
    private let q = DispatchQueue(label: "rm.client")

    // Coalesced motion state; only touched on `q`. Flushing at ~125 Hz caps the
    // packet rate on high-refresh screens without perceptible added latency.
    private var pendingMoveDX = 0
    private var pendingMoveDY = 0
    private var moveFlushScheduled = false
    private let moveInterval: TimeInterval = 0.008

    // Absolute position is latest-wins (not accumulated): only the newest target
    // matters, so a burst of samples collapses to one packet per moveInterval.
    private var pendingAbsX = 0
    private var pendingAbsY = 0
    private var absPending = false
    private var absFlushScheduled = false

    // Plain TCP with Nagle disabled so each small input packet ships immediately.
    private static func makeParams() -> NWParameters {
        let tcp = NWProtocolTCP.Options()
        tcp.noDelay = true
        return NWParameters(tls: nil, tcp: tcp)
    }

    func connect(endpoint: NWEndpoint, password: String) {
        let c = NWConnection(to: endpoint, using: Client.makeParams())
        start(c, password: password)
    }
    func connect(host: String, port: UInt16, password: String) {
        let c = NWConnection(host: .init(host), port: .init(rawValue: port)!, using: Client.makeParams())
        start(c, password: password)
    }

    private func start(_ c: NWConnection, password: String) {
        self.password = password
        conn = c
        set(.connecting)
        c.stateUpdateHandler = { [weak self] st in
            switch st {
            case .ready: self?.send(["t": "hello", "name": UIName.device, "platform": "ios", "ver": "0.1"]); self?.set(.authenticating)
            case .failed(let e): self?.set(.failed(e.localizedDescription))
            case .cancelled: self?.set(.idle)
            default: break
            }
        }
        receive()
        c.start(queue: q)
    }

    func disconnect() {
        q.async { [weak self] in
            guard let self else { return }
            self.flushMove()
            self.flushAbs()
            self.writeLine(["t": "bye"])
            self.conn?.cancel()
            self.conn = nil
        }
    }

    // MARK: events
    // Motion is accumulated and flushed at most once per `moveInterval`; the
    // trailing flush preserves the final position while capping the send rate.
    func move(_ dx: Int, _ dy: Int) {
        q.async { [weak self] in
            guard let self else { return }
            self.pendingMoveDX += dx
            self.pendingMoveDY += dy
            guard !self.moveFlushScheduled else { return }
            self.moveFlushScheduled = true
            self.q.asyncAfter(deadline: .now() + self.moveInterval) { [weak self] in
                self?.flushMove()
            }
        }
    }
    func click(_ b: String) { send(["t": "click", "b": b]) }
    func button(_ b: String, _ down: Bool) { send(["t": "button", "b": b, "down": down]) }
    func scroll(_ dx: Int, _ dy: Int) { send(["t": "scroll", "dx": dx, "dy": dy]) }

    // Absolute pointer position, normalized 0..65535 over the primary screen.
    // Coalesced latest-wins so high-rate motion output caps at one packet per
    // moveInterval without lagging behind the true position.
    func moveAbs(_ nx: Int, _ ny: Int) {
        q.async { [weak self] in
            guard let self else { return }
            self.pendingAbsX = nx
            self.pendingAbsY = ny
            self.absPending = true
            guard !self.absFlushScheduled else { return }
            self.absFlushScheduled = true
            self.q.asyncAfter(deadline: .now() + self.moveInterval) { [weak self] in
                self?.flushAbs()
            }
        }
    }

    // Ask the server for its primary-screen size (reply handled as "screen").
    func requestScreen() { send(["t": "getscreen"]) }

    func text(_ s: String) { send(["t": "text", "s": s]) }
    func key(_ code: Int, mods: Int = 0) {
        send(["t": "key", "code": code, "down": true, "mods": mods])
        send(["t": "key", "code": code, "down": false, "mods": mods])
    }
    func ping() { send(["t": "ping", "ts": Int(Date().timeIntervalSince1970 * 1000)]) }

    // Flush coalesced motion as a single packet. Must run on `q`.
    private func flushMove() {
        moveFlushScheduled = false
        let dx = pendingMoveDX, dy = pendingMoveDY
        pendingMoveDX = 0
        pendingMoveDY = 0
        guard dx != 0 || dy != 0 else { return }
        writeLine(["t": "move", "dx": dx, "dy": dy])
    }

    // Flush the latest absolute position. Must run on `q`.
    private func flushAbs() {
        absFlushScheduled = false
        guard absPending else { return }
        absPending = false
        writeLine(["t": "moveabs", "x": pendingAbsX, "y": pendingAbsY])
    }

    // Serialize non-move sends on `q`, flushing pending motion first so a click
    // or scroll never overtakes the moves that came before it.
    private func send(_ obj: [String: Any]) {
        q.async { [weak self] in
            guard let self else { return }
            self.flushMove()
            self.flushAbs()
            self.writeLine(obj)
        }
    }

    private func writeLine(_ obj: [String: Any]) {
        guard let c = conn, var d = try? JSONSerialization.data(withJSONObject: obj) else { return }
        d.append(0x0a)
        c.send(content: d, completion: .contentProcessed { _ in })
    }

    private func receive() {
        conn?.receive(minimumIncompleteLength: 1, maximumLength: 65536) { [weak self] data, _, done, err in
            guard let self else { return }
            if let data, !data.isEmpty { self.buf.append(data); self.drain() }
            if done || err != nil { return }
            self.receive()
        }
    }

    private func drain() {
        while let i = buf.firstIndex(of: 0x0a) {
            let line = buf.subdata(in: buf.startIndex..<i)
            buf.removeSubrange(buf.startIndex...i)
            guard let m = try? JSONSerialization.jsonObject(with: line) as? [String: Any] else { continue }
            handle(m)
        }
    }

    private func handle(_ m: [String: Any]) {
        switch m["t"] as? String {
        case "challenge":
            guard let s = m["salt"] as? String, let n = m["nonce"] as? String,
                  let iter = m["iter"] as? Int,
                  let salt = Data(base64Encoded: s), let nonce = Data(base64Encoded: n) else { return }
            let p = Crypto.proof(password: password, salt: salt, nonce: nonce, iterations: iter)
            send(["t": "auth", "proof": p])
        case "auth_ok":
            DispatchQueue.main.async { self.serverName = m["server"] as? String ?? "" }
            set(.connected)
        case "screen":
            if let w = m["w"] as? Int, let h = m["h"] as? Int {
                DispatchQueue.main.async { self.screenW = w; self.screenH = h }
            }
        case "error":
            set(.failed(m["msg"] as? String ?? "error"))
        default: break
        }
    }

    private func set(_ s: ClientState) { DispatchQueue.main.async { self.state = s } }
}

enum UIName { static var device: String { "iPhone" } }

// Neutral keycodes/mods mirroring server/keys.go.
enum K {
    static let ctrl = 1, alt = 2, shift = 4, meta = 8
    static let enter = 1, back = 2, tab = 3, esc = 4, del = 5
    static let up = 10, down = 11, left = 12, right = 13
    static let volDown = 200, volUp = 201, mute = 202, playPause = 203, next = 204, prev = 205
}
