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

            // Recent clients section
            if !clientViewModel.activeRecentClients.isEmpty {
                VStack(alignment: .leading) {
                    Text("Recent Clients")
                        .font(.headline)
                        .foregroundColor(Color(hex: "94a3b8"))
                        .padding(.horizontal)

                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 12) {
                            ForEach(clientViewModel.activeRecentClients) { client in
                                RecentClientCard(client: client) {
                                    openTerminal(for: client)
                                }
                            }
                        }
                        .padding(.horizontal)
                    }
                }
                .padding(.vertical, 8)
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
        }
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
        .onChange(of: showingCamera) { _, newValue in
            if !newValue {
                if let camera = currentCamera {
                    clientViewModel.removeCamera(id: camera.id)
                    currentCamera = nil
                }
            }
        }
        .onChange(of: showingTerminal) { _, newValue in
            if !newValue {
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
        // Directly set the selected client, which will trigger the sheet presentation
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

            TextField(placeholder, text: $text)
                .foregroundColor(.white)

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

struct RecentClientCard: View {
    let client: Client
    let action: () -> Void

    var body: some View {
        Button(action: action, label: {
            VStack(alignment: .leading) {
                Text(client.name ?? client.mid)
                    .font(.headline)
                    .foregroundColor(.white)
                    .lineLimit(1)

                Text(client.mid)
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
                    .lineLimit(1)
            }
            .frame(width: 150, height: 80)
            .padding()
            .background(Color(hex: "1e293b"))
            .cornerRadius(10)
        })
    }
}

struct ClientRow: View {
    let client: Client
    let onTerminalTap: () -> Void
    let onCameraTap: () -> Void
    let onPortForwardTap: () -> Void

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 4) {
                Text(client.name ?? client.mid)
                    .font(.headline)
                    .foregroundColor(.white)

                Text(client.mid)
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
            }

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
