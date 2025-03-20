#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import Combine
import CoreGraphics
import Foundation

class AuthViewModel: ObservableObject {
    @Published var isAuthenticated = false
    @Published var token: String?
    @Published var isLoading = false
    @Published var error: String?

    private var cancellables = Set<AnyCancellable>()

    init() {
        // Check for UI testing bypass
        if UITestingHelper.shouldBypassAuth {
            token = UITestingHelper.mockToken
            isAuthenticated = true
            return
        }

        // Check for saved token
        if let savedToken = UserDefaults.standard.string(forKey: "authToken") {
            token = savedToken
            isAuthenticated = true
        }
    }

    func login(username: String, password: String) {
        guard !username.isEmpty, !password.isEmpty else {
            error = "Username and password are required"
            isLoading = false
            return
        }

        isLoading = true
        error = nil

        // Special handling for UI testing
        if UITestingHelper.isUITesting && !UITestingHelper.shouldBypassAuth {
            handleUITestingLogin(username: username, password: password)
            return
        }

        Task { @MainActor in
            // Create the login request
            guard let url = URL(string: "\(APIService.baseURL)/auth/login") else {
                self.error = "Invalid server URL"
                self.isLoading = false
                return
            }

            var request = URLRequest(url: url)
            request.httpMethod = "POST"
            request.addValue("application/json", forHTTPHeaderField: "Content-Type")

            let body: [String: String] = ["username": username, "password": password]
            request.httpBody = try? JSONEncoder().encode(body)

            // Note: For login requests, we don't need to use authHandledDataTaskPublisher
            // since we're not sending an auth token yet
            URLSession.shared.dataTaskPublisher(for: request)
                .map { $0.data }
                .decode(type: StandardResponse<AuthResponse>.self, decoder: JSONDecoder())
                .receive(on: DispatchQueue.main)
                .sink(receiveCompletion: { [weak self] completion in
                    self?.isLoading = false

                    if case let .failure(error) = completion {
                        self?.handleLoginError(error)
                    }
                }, receiveValue: { [weak self] response in
                    self?.token = response.data?.token
                    self?.isAuthenticated = true

                    // Save token
                    UserDefaults.standard.set(response.data?.token, forKey: "authToken")
                })
                .store(in: &cancellables)
        }
    }

    // Handle login for UI testing
    private func handleUITestingLogin(username: String, password: String) {
        // Simulate login for UI testing
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
            if username == "invalid_user" && password == "invalid_password" {
                self.error = "Invalid username or password"
                self.isLoading = false
            } else if username == "test_user" && password == "test_password" {
                self.token = "ui-test-token"
                self.isAuthenticated = true
                UserDefaults.standard.set("ui-test-token", forKey: "authToken")
                self.isLoading = false
            } else {
                // Default behavior for other credentials in UI testing
                self.error = "Invalid username or password"
                self.isLoading = false
            }
        }
    }

    // Handle login errors
    private func handleLoginError(_ error: Error) {
        if let urlError = error as? URLError {
            switch urlError.code {
            case .notConnectedToInternet:
                self.error = "No internet connection"
            case .cannotConnectToHost:
                self.error = "Cannot connect to server"
            case .timedOut:
                self.error = "Connection timed out"
            default:
                self.error = "Network error: \(urlError.localizedDescription)"
            }
        } else {
            self.error = "Invalid username or password"
        }
    }

    func logout() {
        token = nil
        isAuthenticated = false
        UserDefaults.standard.removeObject(forKey: "authToken")
    }
}

struct AuthResponse: Codable {
    let token: String
}
