import SwiftUI

// Compose text on the phone and send it to the PC as a text-injection event.
struct TextInputPane: View {
    @EnvironmentObject var client: Client
    @Environment(\.verticalSizeClass) private var vSize
    @State private var typed = ""
    @FocusState private var focused: Bool

    var body: some View {
        VStack(spacing: 12) {
            TextField("输入文本，点「发送」打到电脑", text: $typed, axis: .vertical)
                .focused($focused)
                .font(.title3)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .padding(14)
                .background(RoundedRectangle(cornerRadius: 16).fill(Color(.secondarySystemBackground)))

            HStack(spacing: 10) {
                Button { typed = "" } label: {
                    Text("清空")
                        .font(.headline)
                        .frame(height: vSize == .compact ? 44 : 52)
                        .padding(.horizontal, 20)
                }
                .buttonStyle(.bordered)
                .disabled(typed.isEmpty)

                Button { send() } label: {
                    Label("发送", systemImage: "paperplane.fill")
                        .font(.headline)
                        .frame(maxWidth: .infinity)
                        .frame(height: vSize == .compact ? 44 : 52)
                }
                .buttonStyle(.borderedProminent)
                .disabled(typed.isEmpty)
            }
        }
        .frame(maxWidth: 700)
        .frame(maxWidth: .infinity)
        .task {
            try? await Task.sleep(nanoseconds: 350_000_000)
            focused = true
        }
    }

    private func send() {
        let s = typed
        guard !s.isEmpty else { return }
        client.text(s)
        typed = ""
    }
}
