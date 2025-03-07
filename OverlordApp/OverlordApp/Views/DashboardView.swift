import SwiftUI

struct DashboardView: View {
    @EnvironmentObject private var authViewModel: AuthViewModel
    @StateObject private var clientViewModel = ClientViewModel()
    @StateObject private var terminalViewModel = TerminalViewModel()
    @StateObject private var uploadProgressViewModel = UploadProgressViewModel()
    @StateObject private var webSocketService = WebSocketService()
    
    @State private var searchText = ""
    @State private var showingLogoutAlert = false
    @State private var selectedTab = 0
    
    var body: some View {
        TabView(selection: $selectedTab) {
            // Clients Tab
            NavigationView {
                ClientsListView(clientViewModel: clientViewModel)
                    .navigationTitle("Clients")
                    .navigationBarItems(
                        trailing: Button(action: {
                            showingLogoutAlert = true
                        }) {
                            Image(systemName: "rectangle.portrait.and.arrow.right")
                                .foregroundColor(Color(hex: "10b981"))
                        }
                    )
            }
            .tabItem {
                Label("Clients", systemImage: "desktopcomputer")
            }
            .tag(0)
            
            // Terminals Tab
            NavigationView {
                TerminalsView(terminalViewModel: terminalViewModel)
                    .navigationTitle("Terminals")
            }
            .tabItem {
                Label("Terminals", systemImage: "terminal")
            }
            .tag(1)
            
            // Cameras Tab
            NavigationView {
                CamerasView(clientViewModel: clientViewModel)
                    .navigationTitle("Cameras")
            }
            .tabItem {
                Label("Cameras", systemImage: "video")
            }
            .tag(2)
            
            // Settings Tab
            NavigationView {
                SettingsView()
                    .navigationTitle("Settings")
            }
            .tabItem {
                Label("Settings", systemImage: "gear")
            }
            .tag(3)
        }
        .accentColor(Color(hex: "10b981"))
        .onAppear {
            setupServices()
        }
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
        clientViewModel.loadInitialClients(token: token)
        clientViewModel.setupWebSocketHandlers()
        terminalViewModel.setupWebSocketHandlers()
        uploadProgressViewModel.setupWebSocketHandlers()
        
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