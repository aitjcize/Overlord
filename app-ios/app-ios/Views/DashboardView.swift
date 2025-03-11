import CoreGraphics
import SwiftTerm
import SwiftUI
import WebKit

// Custom modifier to reduce space above navigation title
struct ReducedNavigationTitleSpacing: ViewModifier {
    func body(content: Content) -> some View {
        content
            .onAppear {
                // Reduce the default spacing above large navigation titles
                let appearance = UINavigationBarAppearance()
                appearance.configureWithOpaqueBackground()
                appearance.backgroundColor = UIColor(Color(hex: "1e293b"))
                appearance.titleTextAttributes = [.foregroundColor: UIColor.white]
                appearance.largeTitleTextAttributes = [.foregroundColor: UIColor.white]

                // Set the spacing to a smaller value
                let style = NSMutableParagraphStyle()
                style.firstLineHeadIndent = 0
                style.headIndent = 0
                style.tailIndent = 0
                style.paragraphSpacing = 0
                style.paragraphSpacingBefore = 0
                appearance.largeTitleTextAttributes[.paragraphStyle] = style

                UINavigationBar.appearance().standardAppearance = appearance
                UINavigationBar.appearance().scrollEdgeAppearance = appearance
                UINavigationBar.appearance().compactAppearance = appearance
            }
    }
}

extension View {
    func reducedNavigationTitleSpacing() -> some View {
        modifier(ReducedNavigationTitleSpacing())
    }
}

struct DashboardView: View {
    @EnvironmentObject private var authViewModel: AuthViewModel
    @StateObject private var clientViewModel = ClientViewModel()
    @StateObject private var webSocketService = WebSocketService()
    @StateObject private var terminalViewModel: TerminalViewModel
    @StateObject private var uploadProgressViewModel = UploadProgressViewModel()
    @StateObject private var portForwardViewModel = PortForwardViewModel()

    init() {
        // Create the WebSocketService first
        let webSocketService = WebSocketService()
        // Then use it to initialize the TerminalViewModel
        _webSocketService = StateObject(wrappedValue: webSocketService)
        _terminalViewModel = StateObject(wrappedValue: TerminalViewModel(webSocketService: webSocketService))
    }

    @State private var searchText = ""
    @State private var showingLogoutAlert = false
    @State private var selectedTab = 0

    var body: some View {
        TabView(selection: $selectedTab) {
            // Clients Tab
            NavigationView {
                ClientsListView(
                    clientViewModel: clientViewModel,
                    terminalViewModel: terminalViewModel,
                    portForwardViewModel: portForwardViewModel
                )
                .navigationTitle("Clients")
                .navigationBarTitleDisplayMode(.large)
                .toolbarColorScheme(.dark, for: .navigationBar)
                .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
                .toolbarBackground(.visible, for: .navigationBar)
            }
            .tabItem {
                Label("Clients", systemImage: "desktopcomputer")
            }
            .tag(0)
            .reducedNavigationTitleSpacing()

            // Terminals Tab
            NavigationView {
                TerminalsView(terminalViewModel: terminalViewModel)
                    .navigationTitle("Terminals")
                    .navigationBarTitleDisplayMode(.large)
                    .toolbarColorScheme(.dark, for: .navigationBar)
                    .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
                    .toolbarBackground(.visible, for: .navigationBar)
            }
            .tabItem {
                Label("Terminals", systemImage: "terminal")
            }
            .tag(1)
            .reducedNavigationTitleSpacing()

            // Port Forwards Tab
            NavigationView {
                PortForwardsView(portForwardViewModel: portForwardViewModel)
                    .navigationTitle("Port Forwards")
                    .navigationBarTitleDisplayMode(.large)
                    .toolbarColorScheme(.dark, for: .navigationBar)
                    .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
                    .toolbarBackground(.visible, for: .navigationBar)
            }
            .tabItem {
                Label("Forwards", systemImage: "network")
            }
            .tag(2)
            .reducedNavigationTitleSpacing()

            // Cameras Tab
            NavigationView {
                CamerasView(clientViewModel: clientViewModel)
                    .navigationTitle("Cameras")
                    .navigationBarTitleDisplayMode(.large)
                    .toolbarColorScheme(.dark, for: .navigationBar)
                    .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
                    .toolbarBackground(.visible, for: .navigationBar)
            }
            .tabItem {
                Label("Cameras", systemImage: "video")
            }
            .tag(3)
            .reducedNavigationTitleSpacing()

            // Settings Tab
            NavigationView {
                SettingsView()
                    .navigationTitle("Settings")
                    .navigationBarTitleDisplayMode(.large)
                    .toolbarColorScheme(.dark, for: .navigationBar)
                    .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
                    .toolbarBackground(.visible, for: .navigationBar)
            }
            .tabItem {
                Label("Settings", systemImage: "gear")
            }
            .tag(4)
            .reducedNavigationTitleSpacing()
        }
        .accentColor(Color(hex: "10b981"))
        .onAppear {
            setupServices()

            // Check if the app was in background and restart all TCP servers if needed
            if OverlordDashboardApp.wasInBackground {
                print("DashboardView: App was in background, restarting all TCP servers")
                // Restart all TCP servers
                portForwardViewModel.restartAllTCPServers()
            }
        }
        .sheet(isPresented: Binding<Bool>(
            get: { portForwardViewModel.shouldShowPortForwardWebView },
            set: { newValue in
                portForwardViewModel.shouldShowPortForwardWebView = newValue
                if !newValue {
                    portForwardViewModel.lastCreatedPortForward = nil
                }
            }
        ), onDismiss: {
            portForwardViewModel.shouldShowPortForwardWebView = false
            portForwardViewModel.lastCreatedPortForward = nil
        }, content: {
            Group {
                if let portForward = portForwardViewModel.lastCreatedPortForward, let url = portForward.localURL {
                    WebViewContainer(
                        url: url,
                        title: portForward.displayName,
                        portForward: portForward,
                        viewModel: portForwardViewModel
                    )
                } else {
                    VStack(spacing: 16) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 50))
                            .foregroundColor(.yellow)

                        Text("Invalid Port Forward")
                            .font(.headline)
                            .foregroundColor(.red)

                        Button("Close") {
                            portForwardViewModel.shouldShowPortForwardWebView = false
                        }
                        .padding()
                        .background(Color.blue)
                        .foregroundColor(.white)
                        .cornerRadius(8)
                    }
                    .padding()
                }
            }
        })
        .alert(isPresented: $showingLogoutAlert) {
            Alert(
                title: Text("Logout"),
                message: Text("Are you sure you want to logout?"),
                primaryButton: .destructive(Text("Logout")) {
                    logout()
                },
                secondaryButton: .cancel()
            )
        }
        .overlay(
            Group {
                if !uploadProgressViewModel.records.isEmpty {
                    UploadProgressOverlay(viewModel: uploadProgressViewModel)
                        .padding()
                        .transition(.move(edge: .bottom))
                }
            }
        )
    }

    private func setupServices() {
        guard let token = authViewModel.token else {
            return
        }

        // Start WebSocket connection
        webSocketService.start(token: token)

        // Set up view models
        Task { @MainActor in
            await clientViewModel.loadInitialClients()
            clientViewModel.startMonitoring()
            terminalViewModel.setupWebSocketHandlers()
            uploadProgressViewModel.setupWebSocketHandlers()
        }

        // Listen for logout requests
        NotificationCenter.default.addObserver(
            forName: .logoutRequested,
            object: nil,
            queue: .main
        ) { _ in
            authViewModel.logout()
        }
    }

    private func logout() {
        // Stop WebSocket connection
        webSocketService.stop()

        // Logout
        authViewModel.logout()
    }
}

struct DashboardView_Previews: PreviewProvider {
    static var previews: some View {
        DashboardView()
            .environmentObject(AuthViewModel())
    }
}
