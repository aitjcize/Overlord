import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var authViewModel: AuthViewModel
    @State private var serverAddress = UserDefaults.standard.string(forKey: "serverAddress") ?? "http://your-server-address"
    @State private var showingLogoutAlert = false
    @State private var showingSaveAlert = false
    
    var body: some View {
        ZStack {
            Color(hex: "0f172a").ignoresSafeArea()
            
            ScrollView {
                VStack(spacing: 20) {
                    // Server settings
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Server Settings")
                            .font(.headline)
                            .foregroundColor(.white)
                        
                        TextField("Server Address", text: $serverAddress)
                            .padding()
                            .background(Color(hex: "1e293b"))
                            .cornerRadius(10)
                            .foregroundColor(.white)
                        
                        Button(action: {
                            saveServerAddress()
                        }) {
                            Text("Save Server Address")
                                .frame(maxWidth: .infinity)
                                .padding()
                                .background(Color(hex: "10b981"))
                                .foregroundColor(.white)
                                .cornerRadius(10)
                        }
                    }
                    .padding()
                    .background(Color(hex: "334155"))
                    .cornerRadius(10)
                    
                    // Account settings
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Account")
                            .font(.headline)
                            .foregroundColor(.white)
                        
                        Button(action: {
                            showingLogoutAlert = true
                        }) {
                            HStack {
                                Text("Logout")
                                    .foregroundColor(.white)
                                
                                Spacer()
                                
                                Image(systemName: "rectangle.portrait.and.arrow.right")
                                    .foregroundColor(Color(hex: "ef4444"))
                            }
                            .padding()
                            .background(Color(hex: "1e293b"))
                            .cornerRadius(10)
                        }
                    }
                    .padding()
                    .background(Color(hex: "334155"))
                    .cornerRadius(10)
                    
                    // About section
                    VStack(alignment: .leading, spacing: 12) {
                        Text("About")
                            .font(.headline)
                            .foregroundColor(.white)
                        
                        VStack(alignment: .leading, spacing: 8) {
                            InfoRow(label: "App Version", value: "1.0.0")
                            InfoRow(label: "Build", value: "1")
                            InfoRow(label: "Device", value: UIDevice.current.model)
                            InfoRow(label: "System", value: "\(UIDevice.current.systemName) \(UIDevice.current.systemVersion)")
                        }
                        .padding()
                        .background(Color(hex: "1e293b"))
                        .cornerRadius(10)
                    }
                    .padding()
                    .background(Color(hex: "334155"))
                    .cornerRadius(10)
                }
                .padding()
            }
        }
        .alert(isPresented: $showingLogoutAlert) {
            Alert(
                title: Text("Logout"),
                message: Text("Are you sure you want to logout?"),
                primaryButton: .destructive(Text("Logout")) {
                    authViewModel.logout()
                },
                secondaryButton: .cancel()
            )
        }
        .alert(isPresented: $showingSaveAlert) {
            Alert(
                title: Text("Settings Saved"),
                message: Text("Server address has been updated. The app will use the new address for future connections."),
                dismissButton: .default(Text("OK"))
            )
        }
    }
    
    private func saveServerAddress() {
        UserDefaults.standard.set(serverAddress, forKey: "serverAddress")
        APIService.baseURL = serverAddress + "/api"
        showingSaveAlert = true
    }
}

struct InfoRow: View {
    let label: String
    let value: String
    
    var body: some View {
        HStack {
            Text(label)
                .foregroundColor(Color(hex: "94a3b8"))
            
            Spacer()
            
            Text(value)
                .foregroundColor(.white)
        }
    }
}

struct SettingsView_Previews: PreviewProvider {
    static var previews: some View {
        SettingsView()
            .environmentObject(AuthViewModel())
    }
} 