#if os(iOS)
    import UIKit
#endif
import CoreGraphics
import SwiftUI

// UIKit TextField wrapper for better placeholder visibility
struct CustomTextField: UIViewRepresentable {
    @Binding var text: String
    let placeholder: String
    let keyboardType: UIKeyboardType
    let returnKeyType: UIReturnKeyType
    let isSecure: Bool
    let accessibilityIdentifier: String

    init(
        text: Binding<String>,
        placeholder: String,
        keyboardType: UIKeyboardType = .default,
        returnKeyType: UIReturnKeyType = .default,
        isSecure: Bool = false,
        accessibilityIdentifier: String = ""
    ) {
        _text = text
        self.placeholder = placeholder
        self.keyboardType = keyboardType
        self.returnKeyType = returnKeyType
        self.isSecure = isSecure
        self.accessibilityIdentifier = accessibilityIdentifier
    }

    func makeUIView(context: Context) -> UITextField {
        let textField = UITextField()
        textField.delegate = context.coordinator
        textField.placeholder = placeholder
        textField.keyboardType = keyboardType
        textField.returnKeyType = returnKeyType
        textField.isSecureTextEntry = isSecure
        textField.autocorrectionType = .no
        textField.autocapitalizationType = .none
        textField.textColor = .white
        textField.font = UIFont.systemFont(ofSize: 16) // Set appropriate font size
        textField.backgroundColor = .clear // Make background clear
        textField.accessibilityIdentifier = accessibilityIdentifier

        // Set placeholder color to a subtle gray that works well on dark backgrounds
        textField.attributedPlaceholder = NSAttributedString(
            string: placeholder,
            attributes: [NSAttributedString.Key.foregroundColor: UIColor(white: 0.7, alpha: 0.7)]
        )

        return textField
    }

    func updateUIView(_ uiView: UITextField, context _: Context) {
        uiView.text = text
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(text: $text)
    }

    class Coordinator: NSObject, UITextFieldDelegate {
        @Binding var text: String

        init(text: Binding<String>) {
            _text = text
        }

        func textFieldDidChangeSelection(_ textField: UITextField) {
            text = textField.text ?? ""
        }
    }
}

struct LoginView: View {
    @EnvironmentObject private var authViewModel: AuthViewModel
    @State private var username: String = ""
    @State private var password: String = ""
    @State private var serverHost: String = ""

    // Initialize serverHost from UserDefaults, but strip protocol prefix for display
    init() {
        let savedAddress = UserDefaults.standard.string(forKey: "serverAddress") ?? "http://localhost:8080"
        // Strip http:// or https:// for display
        let displayAddress = savedAddress
            .replacingOccurrences(of: "https://", with: "")
            .replacingOccurrences(of: "http://", with: "")
        _serverHost = State(initialValue: displayAddress)
    }

    var body: some View {
        ZStack {
            // Background gradient
            LinearGradient(
                gradient: Gradient(colors: [Color(hex: "0f172a"), Color(hex: "1e293b"), Color(hex: "334155")]),
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
            .ignoresSafeArea()

            VStack(spacing: 30) {
                // Logo and title
                VStack(spacing: 10) {
                    Image(systemName: "network")
                        .font(.system(size: 60))
                        .foregroundColor(Color(hex: "10b981"))

                    Text("Overlord Dashboard")
                        .font(.largeTitle)
                        .fontWeight(.bold)
                        .foregroundColor(.white)

                    Text("Mobile Client")
                        .font(.headline)
                        .foregroundColor(Color(hex: "94a3b8"))
                }

                // Login form
                VStack(spacing: 20) {
                    CustomTextField(
                        text: $serverHost,
                        placeholder: "Server Host (e.g. example.com:9000)",
                        keyboardType: .URL,
                        returnKeyType: .next,
                        accessibilityIdentifier: "Server Address"
                    )
                    .frame(height: 44) // Control the height
                    .padding(.horizontal, 12)
                    .background(Color(hex: "2a3f5a"))
                    .cornerRadius(8)

                    CustomTextField(
                        text: $username,
                        placeholder: "Username",
                        returnKeyType: .next,
                        accessibilityIdentifier: "Username"
                    )
                    .frame(height: 44) // Control the height
                    .padding(.horizontal, 12)
                    .background(Color(hex: "2a3f5a"))
                    .cornerRadius(8)

                    CustomTextField(
                        text: $password,
                        placeholder: "Password",
                        returnKeyType: .done,
                        isSecure: true,
                        accessibilityIdentifier: "Password"
                    )
                    .frame(height: 44) // Control the height
                    .padding(.horizontal, 12)
                    .background(Color(hex: "2a3f5a"))
                    .cornerRadius(8)

                    if let error = authViewModel.error {
                        Text(error)
                            .foregroundColor(.red)
                            .font(.caption)
                            .accessibilityIdentifier("ErrorMessage")
                    }

                    Button(
                        action: {
                            // Start loading state
                            authViewModel.isLoading = true

                            // Process server host input asynchronously
                            Task { @MainActor in
                                let fullServerAddress = await processServerHostAsync(serverHost)

                                // Save server address
                                UserDefaults.standard.set(fullServerAddress, forKey: "serverAddress")

                                // Update API base URL
                                APIService.baseURL = fullServerAddress + "/api"

                                // Login
                                authViewModel.login(username: username, password: password)
                            }
                        },
                        label: {
                            HStack {
                                Text("Login")
                                    .fontWeight(.semibold)

                                if authViewModel.isLoading {
                                    ProgressView()
                                        .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                        .padding(.leading, 5)
                                }
                            }
                            .frame(maxWidth: .infinity)
                            .padding()
                            .background(Color(hex: "10b981"))
                            .foregroundColor(.white)
                            .cornerRadius(8)
                        }
                    )
                    .accessibilityIdentifier("Login")
                    .disabled(authViewModel.isLoading)
                }
                .padding(.horizontal, 30)
            }
            .padding(.vertical, 50)
        }
    }

    // Function to process server host input asynchronously
    private func processServerHostAsync(_ host: String) async -> String {
        // If host already includes http:// or https://, return as is
        if host.hasPrefix("http://") || host.hasPrefix("https://") {
            return host
        }

        // Check if the host is available via HTTPS
        let httpsHost = "https://" + host

        // Create a URL request to check HTTPS availability
        guard let httpsURL = URL(string: httpsHost) else {
            return "http://" + host
        }

        var request = URLRequest(url: httpsURL)
        request.httpMethod = "HEAD"
        request.timeoutInterval = 2 // Short timeout for quick check

        // Try HTTPS with a timeout
        do {
            _ = try await URLSession.shared.data(for: request)
            return httpsHost
        } catch {
            print("HTTPS not available for \(host), using HTTP instead: \(error.localizedDescription)")
            return "http://" + host
        }
    }
}

struct LoginView_Previews: PreviewProvider {
    static var previews: some View {
        LoginView()
            .environmentObject(AuthViewModel())
    }
}
