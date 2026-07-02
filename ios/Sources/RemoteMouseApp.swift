import SwiftUI

@main
struct RemoteMouseApp: App {
    @StateObject private var client = Client()
    var body: some Scene {
        WindowGroup {
            RootView().environmentObject(client)
        }
    }
}

struct RootView: View {
    @EnvironmentObject var client: Client
    var body: some View {
        if client.state == .connected {
            ControlView()
        } else {
            DiscoveryView()
        }
    }
}
