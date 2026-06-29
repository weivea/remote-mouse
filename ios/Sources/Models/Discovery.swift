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
    private var browser: NWBrowser?

    func start() {
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
    }

    func stop() { browser?.cancel(); browser = nil }
}
