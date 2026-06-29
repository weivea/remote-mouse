import Foundation
import Network

enum ClientState: Equatable {
    case idle, connecting, authenticating, connected, failed(String)
}

final class Client: ObservableObject {
    @Published var state: ClientState = .idle
    @Published var serverName: String = ""

    private var conn: NWConnection?
    private var password = ""
    private var buf = Data()
    private let q = DispatchQueue(label: "rm.client")

    func connect(endpoint: NWEndpoint, password: String) {
        let c = NWConnection(to: endpoint, using: .tcp)
        start(c, password: password)
    }
    func connect(host: String, port: UInt16, password: String) {
        let c = NWConnection(host: .init(host), port: .init(rawValue: port)!, using: .tcp)
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

    func disconnect() { send(["t": "bye"]); conn?.cancel(); conn = nil }

    // MARK: events
    func move(_ dx: Int, _ dy: Int) { send(["t": "move", "dx": dx, "dy": dy]) }
    func click(_ b: String) { send(["t": "click", "b": b]) }
    func button(_ b: String, _ down: Bool) { send(["t": "button", "b": b, "down": down]) }
    func scroll(_ dx: Int, _ dy: Int) { send(["t": "scroll", "dx": dx, "dy": dy]) }
    func text(_ s: String) { send(["t": "text", "s": s]) }
    func ping() { send(["t": "ping", "ts": Int(Date().timeIntervalSince1970 * 1000)]) }

    private func send(_ obj: [String: Any]) {
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
        case "error":
            set(.failed(m["msg"] as? String ?? "error"))
        default: break
        }
    }

    private func set(_ s: ClientState) { DispatchQueue.main.async { self.state = s } }
}

enum UIName { static var device: String { "iPhone" } }
