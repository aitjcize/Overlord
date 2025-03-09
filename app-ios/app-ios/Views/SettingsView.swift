import CoreGraphics
import SwiftUI

#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif

struct SettingsView: View {
    @EnvironmentObject private var authViewModel: AuthViewModel
    @State private var showingLogoutAlert = false

    private var deviceInfo: (model: String, system: String) {
        #if os(iOS)
            return (UIDevice.current.model, "\(UIDevice.current.systemName) \(UIDevice.current.systemVersion)")
        #elseif os(macOS)
            return ("Mac", "macOS")
        #else
            return ("Unknown", "Unknown")
        #endif
    }

    var body: some View {
        ZStack {
            Color(hex: "0f172a").ignoresSafeArea()

            ScrollView {
                VStack(spacing: 20) {
                    // Account settings
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Account")
                            .font(.headline)
                            .foregroundColor(.white)

                        Button(
                            action: {
                                showingLogoutAlert = true
                            },
                            label: {
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
                        )
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
                            InfoRow(label: "Device", value: deviceInfo.model)
                            InfoRow(label: "System", value: deviceInfo.system)
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
        .alert(
            "Logout",
            isPresented: $showingLogoutAlert,
            actions: {
                Button("Cancel", role: .cancel) {}
                Button("Logout", role: .destructive) {
                    authViewModel.logout()
                }
            },
            message: {
                Text("Are you sure you want to logout?")
            }
        )
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
