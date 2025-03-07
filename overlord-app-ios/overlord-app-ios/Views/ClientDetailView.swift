import CoreGraphics
import SwiftUI

struct ClientDetailView: View {
    let client: Client
    @ObservedObject var clientViewModel: ClientViewModel
    @State private var showingTerminal = false
    @State private var showingCamera = false
    @State private var currentCamera: Camera?
    @State private var currentTerminal: Terminal?
    @State private var isLoadingProperties = false
    @Environment(\.presentationMode) var presentationMode
    @StateObject private var terminalViewModel = TerminalViewModel(webSocketService: WebSocketService())

    // Initialize WebSocketService for the terminal
    init(client: Client, clientViewModel: ClientViewModel) {
        self.client = client
        self.clientViewModel = clientViewModel
        // Ensure the client has properties loaded
        if client.properties == nil {
            print("Warning: Client properties are nil for \(client.mid)")
            // Properties will be loaded in onAppear
        }
    }

    var body: some View {
        ZStack {
            Color(hex: "0f172a").ignoresSafeArea()

            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    // Action buttons
                    HStack(spacing: 15) {
                        ActionButton(
                            title: "Terminal",
                            icon: "terminal",
                            color: Color(hex: "10b981")
                        ) {
                            // Create terminal before showing the sheet, but don't add it to the collection yet
                            currentTerminal = terminalViewModel.prepareTerminal(
                                for: client.mid,
                                title: client.name ?? client.mid
                            )
                            showingTerminal = true
                        }

                        if client.hasCamera {
                            ActionButton(
                                title: "Camera",
                                icon: "video",
                                color: Color(hex: "3b82f6")
                            ) {
                                showingCamera = true
                            }
                        }

                        ActionButton(
                            title: "Pin",
                            icon: client.pinned ? "pin.slash" : "pin",
                            color: Color(hex: "f59e0b")
                        ) {
                            clientViewModel.togglePinStatus(for: client.mid)
                        }
                    }
                    .padding(.vertical)

                    // Properties section
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Properties")
                            .font(.headline)
                            .foregroundColor(.white)
                            .padding(.bottom, 4)

                        if isLoadingProperties {
                            HStack {
                                Spacer()
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    .scaleEffect(1.2)
                                Text("Loading properties...")
                                    .foregroundColor(Color(hex: "94a3b8"))
                                    .padding(.leading, 8)
                                Spacer()
                            }
                            .padding()
                        } else if let properties = client.properties, !properties.isEmpty {
                            ForEach(properties.sorted(by: { $0.key < $1.key }), id: \.key) { key, value in
                                PropertyRow(key: key, value: value)
                            }
                        } else {
                            Text("No properties available")
                                .foregroundColor(Color(hex: "94a3b8"))
                                .italic()
                        }
                    }
                    .padding()
                    .background(Color(hex: "1e293b"))
                    .cornerRadius(10)
                }
                .padding()
            }
        }
        .navigationTitle(client.name ?? client.mid)
        .navigationBarTitleDisplayMode(.inline)
        .toolbarColorScheme(.dark, for: .navigationBar)
        .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
        .toolbarBackground(.visible, for: .navigationBar)
        .toolbar {
            ToolbarItem(placement: .navigationBarTrailing) {
                Button(
                    action: {
                        presentationMode.wrappedValue.dismiss()
                    },
                    label: {
                        Image(systemName: "xmark.circle.fill")
                            .foregroundColor(Color(hex: "64748b"))
                            .imageScale(.large)
                    }
                )
            }
        }
        .sheet(isPresented: $showingTerminal) {
            if let terminal = currentTerminal {
                TerminalView(
                    terminal: terminal,
                    terminalViewModel: terminalViewModel
                )
                .onAppear {
                    // Now that the sheet is presented, add the terminal to the collection
                    terminalViewModel.addTerminal(terminal)
                }
            }
        }
        .sheet(isPresented: $showingCamera) {
            if let camera = currentCamera {
                CameraView(camera: camera, clientViewModel: clientViewModel)
            }
        }
        .onChange(of: showingCamera) {
            if !showingCamera {
                if let camera = currentCamera {
                    clientViewModel.removeCamera(id: camera.id)
                    currentCamera = nil
                }
            } else if currentCamera == nil && client.hasCamera {
                currentCamera = clientViewModel.addCamera(id: UUID().uuidString, clientId: client.mid)
            }
        }
        .onChange(of: showingTerminal) {
            if !showingTerminal {
                // Clean up terminal when sheet is dismissed
                if let terminal = currentTerminal {
                    // Close the terminal to clean up resources
                    terminalViewModel.closeTerminal(id: terminal.id)
                    currentTerminal = nil
                }
            }
        }
        .onAppear {
            // If properties are nil, try to load them
            if client.properties == nil {
                loadClientProperties()
            }
        }
    }

    private func loadClientProperties() {
        isLoadingProperties = true

        Task { @MainActor in
            if let token = UserDefaults.standard.string(forKey: "authToken") {
                do {
                    let properties = try await clientViewModel.apiService.getClientProperties(
                        mid: client.mid,
                        token: token
                    ).async()

                    // Update the client with properties
                    var updatedClient = client
                    updatedClient.properties = properties

                    // Update in the view model
                    clientViewModel.clients[client.mid] = updatedClient

                    // Verify the client was updated in the view model
                    if clientViewModel.clients[client.mid] != nil {
                        // We can't directly update 'client' as it's a let constant,
                        // but we've updated it in the view model which will trigger UI updates
                    }
                } catch {
                    print("Failed to load properties for client \(client.mid): \(error)")
                }

                isLoadingProperties = false
            }
        }
    }
}

struct ActionButton: View {
    let title: String
    let icon: String
    let color: Color
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(spacing: 8) {
                Image(systemName: icon)
                    .font(.system(size: 24))
                    .foregroundColor(color)

                Text(title)
                    .font(.caption)
                    .foregroundColor(.white)
            }
            .frame(maxWidth: .infinity)
            .padding()
            .background(Color(hex: "1e293b"))
            .cornerRadius(10)
        }
    }
}

struct PropertyRow: View {
    let key: String
    let value: String

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(key)
                .font(.caption)
                .foregroundColor(Color(hex: "94a3b8"))

            Text(value)
                .font(.body)
                .foregroundColor(.white)
        }
        .padding(.vertical, 4)
        .padding(.horizontal, 8)
        .background(Color(hex: "334155"))
        .cornerRadius(6)
    }
}

struct ClientDetailView_Previews: PreviewProvider {
    static var previews: some View {
        ClientDetailView(
            client: Client(
                mid: "client123",
                name: "Test Client",
                properties: ["os": "Linux", "version": "1.0.0", "has_camera": "true"]
            ),
            clientViewModel: ClientViewModel()
        )
    }
}
