import SwiftUI

struct PortForwardDialog: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var portForwardViewModel: PortForwardViewModel
    let client: Client

    @State private var remoteHost: String = "127.0.0.1"
    @State private var remotePort: String = "80"
    @State private var useHttps: Bool = false
    @State private var isCreating: Bool = false
    @State private var customName: String = ""

    // Computed property for the default name
    private var defaultName: String {
        let clientName = client.name ?? client.mid
        return "\(clientName) - \(remoteHost):\(remotePort)\(useHttps ? " (https)" : "")"
    }

    var body: some View {
        NavigationView {
            Form {
                Section(header: Text("Port Forward Session Name")) {
                    TextField(defaultName, text: $customName)
                        .autocapitalization(.none)
                        .disableAutocorrection(true)
                        .onChange(of: remoteHost) { _ in updatePlaceholder() }
                        .onChange(of: remotePort) { _ in updatePlaceholder() }
                        .onChange(of: useHttps) { _ in updatePlaceholder() }
                        .placeholder(when: customName.isEmpty) {
                            Text(defaultName).foregroundColor(.gray.opacity(0.5))
                        }
                }

                Section(header: Text("Remote Connection Details")) {
                    TextField("Remote Host", text: $remoteHost)
                        .autocapitalization(.none)
                        .disableAutocorrection(true)
                        .keyboardType(.asciiCapable)

                    TextField("Remote Port", text: $remotePort)
                        .keyboardType(.numberPad)

                    Toggle("Use HTTPS", isOn: $useHttps)
                        .onChange(of: useHttps) {
                            // If HTTPS is enabled and port is still default HTTP port, change to HTTPS port
                            if useHttps && remotePort == "80" {
                                remotePort = "443"
                            }
                            // If HTTPS is disabled and port is default HTTPS port, change to HTTP port
                            else if !useHttps && remotePort == "443" {
                                remotePort = "80"
                            }
                        }
                }

                Section(
                    header: Text("Information"),
                    footer: Text(
                        "The port will be forwarded to a local port on your device. " +
                            "You'll be able to access it via localhost."
                    )
                ) {
                    HStack {
                        Text("Client")
                        Spacer()
                        Text(client.name ?? client.mid)
                            .foregroundColor(.secondary)
                    }
                }
            }
            .navigationTitle("Port Forwarding")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") {
                        dismiss()
                    }
                }

                ToolbarItem(placement: .confirmationAction) {
                    Button("Forward") {
                        createPortForward()
                    }
                    .disabled(isCreating || !isValidInput)
                }
            }
            .onAppear {
                print("PortForwardDialog appeared for client: \(client.mid), name: \(client.name ?? "unnamed")")
            }
        }
    }

    private var isValidInput: Bool {
        guard !remoteHost.isEmpty, !remotePort.isEmpty else {
            return false
        }

        guard let port = Int(remotePort), port > 0, port <= 65535 else {
            return false
        }

        return true
    }

    private func updatePlaceholder() {
        // This function is called when any of the inputs change to update the placeholder
        // The actual update happens automatically through the defaultName computed property
    }

    private func createPortForward() {
        guard let port = Int(remotePort) else { return }

        isCreating = true
        print("Creating port forward for client \(client.mid) to \(remoteHost):\(remotePort) (HTTPS: \(useHttps))")

        // Create the port forward with the custom name (if provided)
        _ = portForwardViewModel.createPortForward(
            for: client,
            remoteHost: remoteHost,
            remotePort: port,
            useHttps: useHttps,
            customName: customName.isEmpty ? nil : customName
        )

        isCreating = false
        dismiss()
    }
}

// Extension to add placeholder text to TextField
extension View {
    func placeholder<Content: View>(
        when shouldShow: Bool,
        alignment: Alignment = .leading,
        @ViewBuilder placeholder: () -> Content
    ) -> some View {
        ZStack(alignment: alignment) {
            placeholder().opacity(shouldShow ? 1 : 0)
            self
        }
    }
}
