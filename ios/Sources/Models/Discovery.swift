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
    /// 浏览器状态/权限诊断信息，供界面显示（例如本地网络权限被拒绝）。
    @Published var statusText = ""
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
        statusText = ""
        isScanning = true

        let params = NWParameters.tcp
        params.includePeerToPeer = true
        let b = NWBrowser(for: .bonjour(type: "_remotemouse._tcp", domain: nil), using: params)
        b.browseResultsChangedHandler = { [weak self] results, _ in
            NSLog("[Discovery] browseResults count=\(results.count)")
            let list = results.map { r -> DiscoveredServer in
                var name = "RemoteMouse"
                if case let .service(svcName, _, _, _) = r.endpoint { name = svcName }
                return DiscoveredServer(id: "\(r.endpoint)", name: name, endpoint: r.endpoint)
            }
            DispatchQueue.main.async { self?.servers = list }
        }
        b.stateUpdateHandler = { [weak self] state in
            switch state {
            case .setup:
                NSLog("[Discovery] browser state=setup")
            case .ready:
                NSLog("[Discovery] browser state=ready")
                DispatchQueue.main.async { self?.statusText = "" }
            case .waiting(let error):
                NSLog("[Discovery] browser state=waiting error=\(error)")
                DispatchQueue.main.async {
                    self?.statusText = "网络等待中：\(error.localizedDescription)（可能是本地网络权限未授权，请到 设置→隐私与安全性→本地网络 打开 RemoteMouse）"
                }
            case .failed(let error):
                NSLog("[Discovery] browser state=failed error=\(error)")
                DispatchQueue.main.async {
                    self?.statusText = "浏览失败：\(error.localizedDescription)（请检查 设置→隐私与安全性→本地网络 是否允许 RemoteMouse）"
                    self?.isScanning = false
                }
            case .cancelled:
                NSLog("[Discovery] browser state=cancelled")
            @unknown default:
                NSLog("[Discovery] browser state=unknown")
            }
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
