import Foundation
import Network
import Combine

struct DiscoveredServer: Identifiable, Hashable {
    let id: String
    let name: String
    let endpoint: NWEndpoint
}

final class Discovery: ObservableObject {
    @Published var servers: [DiscoveredServer] = []
    @Published var isScanning = false
    private var browser: NWBrowser?
    private var scanResetWork: DispatchWorkItem?

    func start() {
        restart()
    }

    /// 重新扫描：取消当前 browser、清空已发现列表并重建，不保留任何缓存结果。
    func refresh() {
        restart()
    }

    private func restart() {
        scanResetWork?.cancel()
        browser?.cancel()
        browser = nil

        servers = []
        isScanning = true

        let params = NWParameters.tcp
        params.includePeerToPeer = true
        let b = NWBrowser(for: .bonjour(type: "_remotemouse._tcp", domain: nil), using: params)
        b.browseResultsChangedHandler = { [weak self] results, _ in
            let list = results.map { r -> DiscoveredServer in
                var name = "RemoteMouse"
                if case let .service(svcName, _, _, _) = r.endpoint { name = svcName }
                return DiscoveredServer(id: "\(r.endpoint)", name: name, endpoint: r.endpoint)
            }
            DispatchQueue.main.async { self?.servers = list }
        }
        b.start(queue: .main)
        browser = b

        // 给一个短暂的扫描窗口用于刷新反馈，之后结果仍会实时更新。
        let work = DispatchWorkItem { [weak self] in self?.isScanning = false }
        scanResetWork = work
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.5, execute: work)
    }

    func stop() {
        scanResetWork?.cancel()
        scanResetWork = nil
        browser?.cancel()
        browser = nil
        isScanning = false
    }
}
