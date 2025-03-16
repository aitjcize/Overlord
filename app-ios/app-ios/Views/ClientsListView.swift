import CoreGraphics
import SwiftTerm
import SwiftUI

struct ClientsListView: View {
    @ObservedObject var clientViewModel: ClientViewModel
    @ObservedObject var terminalViewModel: TerminalViewModel
    @ObservedObject var portForwardViewModel: PortForwardViewModel
    @State private var searchText = ""
    @State private var showingTerminal = false
    @State private var showingCamera = false
    @State private var showingPortForwardDialog = false
    @State private var currentCamera: Camera?
    @State private var currentTerminal: Terminal?
    @State private var selectedClient: Client?
    @State private var selectedClientForPortForward: Client?

    var body: some View {
        VStack {
            // Search bar
            SearchBar(text: $searchText, placeholder: "Search clients...")
                .padding(.horizontal)
                .padding(.top, 12)
                .padding(.bottom, 4)
                .onChange(of: searchText) { _, newValue in
                    clientViewModel.setFilterPattern(newValue)
                }

            // All clients list with pinned items at the top
            List {
                ForEach(clientViewModel.filteredClients) { client in
                    ClientRow(
                        client: client,
                        onTerminalTap: {
                            openTerminal(for: client)
                        },
                        onCameraTap: {
                            openCamera(for: client)
                        },
                        onPortForwardTap: {
                            openPortForwardDialog(for: client)
                        }
                    )
                    .contentShape(Rectangle())
                    .swipeActions(
                        edge: .trailing,
                        allowsFullSwipe: true,
                        content: {
                            Button(
                                action: {
                                    clientViewModel.togglePinStatus(for: client.mid)
                                },
                                label: {
                                    Label(
                                        client.pinned ? "Unpin" : "Pin",
                                        systemImage: client.pinned ? "pin.slash.fill" : "pin.fill"
                                    )
                                }
                            )
                            .tint(Color(hex: "f59e0b"))
                        }
                    )
                }
            }
            .listStyle(PlainListStyle())
            .refreshable {
                if let token = UserDefaults.standard.string(forKey: "authToken") {
                    do {
                        let clientsList = try await clientViewModel.apiService.getClients(token: token).async()
                        // Process clients sequentially
                        for client in clientsList {
                            await clientViewModel.loadClientProperties(client: client, token: token)
                        }
                    } catch {
                        print("Failed to refresh clients: \(error)")
                    }
                }
            }
            .onAppear {
                // Configure refresh control appearance
                UIRefreshControl.appearance().tintColor = UIColor(hex: "94a3b8")
                let attributes: [NSAttributedString.Key: Any] = [
                    .foregroundColor: UIColor(hex: "94a3b8")
                ]
                UIRefreshControl.appearance().attributedTitle = NSAttributedString(
                    string: "Pull to refresh",
                    attributes: attributes
                )

                // Start WebSocket monitoring and load initial clients
                Task {
                    await clientViewModel.loadInitialClients()
                    clientViewModel.startMonitoring()
                }
            }
            .onDisappear {
                // Stop WebSocket monitoring when view disappears
                clientViewModel.stopMonitoring()
            }
        }
        .tint(Color(hex: "3b82f6"))
        .accentColor(Color(hex: "3b82f6"))
        .background(Color(hex: "0f172a").ignoresSafeArea())
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
        .sheet(item: $selectedClientForPortForward) { client in
            PortForwardDialog(portForwardViewModel: portForwardViewModel, client: client)
        }
        .onChange(of: showingCamera) {
            if !showingCamera {
                if let camera = currentCamera {
                    clientViewModel.removeCamera(id: camera.id)
                    currentCamera = nil
                }
            }
        }
        .onChange(of: showingTerminal) {
            if !showingTerminal {
                // Clean up terminal when sheet is dismissed
                if currentTerminal != nil {
                    // Don't close the terminal here, just reset the current terminal
                    currentTerminal = nil
                }
            }
        }
    }

    private func openTerminal(for client: Client) {
        let terminal = terminalViewModel.prepareTerminal(
            for: client.mid,
            title: client.name ?? client.mid
        )
        currentTerminal = terminal
        showingTerminal = true
    }

    private func openCamera(for client: Client) {
        let camera = Camera(
            id: UUID().uuidString,
            clientId: client.mid
        )
        currentCamera = camera
        showingCamera = true
    }

    private func openPortForwardDialog(for client: Client) {
        selectedClientForPortForward = client
    }
}

struct SearchBar: View {
    @Binding var text: String
    var placeholder: String

    var body: some View {
        HStack {
            Image(systemName: "magnifyingglass")
                .foregroundColor(Color(hex: "64748b"))

            ZStack(alignment: .leading) {
                if text.isEmpty {
                    Text(placeholder)
                        .foregroundColor(Color(hex: "94a3b8")) // Lighter color for placeholder
                }

                TextField("", text: $text)
                    .foregroundColor(.white)
            }

            if !text.isEmpty {
                Button(
                    action: {
                        text = ""
                    },
                    label: {
                        Image(systemName: "xmark.circle.fill")
                            .foregroundColor(Color(hex: "64748b"))
                    }
                )
            }
        }
        .padding(12)
        .background(Color(hex: "1e293b"))
        .cornerRadius(10)
    }
}

struct ClientRow: View {
    let client: Client
    let onTerminalTap: () -> Void
    let onCameraTap: () -> Void
    let onPortForwardTap: () -> Void

    var body: some View {
        HStack {
            Text(client.name ?? client.mid)
                .font(.headline)
                .foregroundColor(.white)

            Spacer()

            // Port Forward button
            Button(
                action: {
                    onPortForwardTap()
                },
                label: {
                    Image(systemName: "network")
                        .foregroundColor(Color(hex: "3b82f6"))
                        .font(.system(size: 18))
                        .padding(8)
                        .background(Color(hex: "334155"))
                        .cornerRadius(8)
                }
            )
            .buttonStyle(BorderlessButtonStyle())
            .padding(.trailing, 4)

            // Terminal button
            Button(
                action: onTerminalTap,
                label: {
                    Image(systemName: "terminal")
                        .foregroundColor(Color(hex: "10b981"))
                        .font(.system(size: 18))
                        .padding(8)
                        .background(Color(hex: "334155"))
                        .cornerRadius(8)
                }
            )
            .buttonStyle(BorderlessButtonStyle())
            .padding(.trailing, 4)

            // Camera button (only if client has camera)
            if client.hasCamera {
                Button(
                    action: onCameraTap,
                    label: {
                        Image(systemName: "video.fill")
                            .foregroundColor(Color(hex: "3b82f6"))
                            .font(.system(size: 18))
                            .padding(8)
                            .background(Color(hex: "334155"))
                            .cornerRadius(8)
                    }
                )
                .buttonStyle(BorderlessButtonStyle())
                .padding(.trailing, 4)
            }

            // Pin indicator
            if client.pinned {
                Image(systemName: "pin.fill")
                    .foregroundColor(Color(hex: "f59e0b"))
                    .padding(.trailing, 8)
            }
        }
        .padding(.vertical, 8)
        .listRowBackground(Color(hex: "1e293b"))
    }
}

struct ClientsListView_Previews: PreviewProvider {
    static var previews: some View {
        let webSocketService = WebSocketService()
        ClientsListView(
            clientViewModel: ClientViewModel(),
            terminalViewModel: TerminalViewModel(webSocketService: webSocketService),
            portForwardViewModel: PortForwardViewModel()
        )
        .preferredColorScheme(.dark)
    }
}
