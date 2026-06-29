import SwiftUI
import Network

struct DiscoveryView: View {
    @EnvironmentObject var client: Client
    @StateObject private var discovery = Discovery()
    @State private var password = "1234"
    @State private var manualIP = ""
    @State private var manualPort = "27500"

    var body: some View {
        NavigationStack {
            Form {
                Section("密码") {
                    SecureField("连接密码", text: $password)
                        .textContentType(.password)
                }
                Section("发现的电脑") {
                    if discovery.servers.isEmpty {
                        Text("搜索中…同一 WiFi 下应自动出现").foregroundStyle(.secondary)
                    }
                    ForEach(discovery.servers) { s in
                        Button { client.connect(endpoint: s.endpoint, password: password) } label: {
                            HStack { Image(systemName: "desktopcomputer"); Text(s.name); Spacer()
                                Image(systemName: "chevron.right").foregroundStyle(.secondary) }
                        }
                    }
                }
                Section("手动连接") {
                    TextField("IP 地址", text: $manualIP).keyboardType(.decimalPad)
                    TextField("端口", text: $manualPort).keyboardType(.numberPad)
                    Button("连接") {
                        if let p = UInt16(manualPort), !manualIP.isEmpty {
                            client.connect(host: manualIP, port: p, password: password)
                        }
                    }.disabled(manualIP.isEmpty)
                }
                if case .failed(let m) = client.state {
                    Text("连接失败：\(m)").foregroundStyle(.red)
                } else if client.state != .idle {
                    Text("连接中…").foregroundStyle(.secondary)
                }
            }
            .navigationTitle("Remote Mouse")
        }
        .onAppear { discovery.start() }
        .onDisappear { discovery.stop() }
    }
}
