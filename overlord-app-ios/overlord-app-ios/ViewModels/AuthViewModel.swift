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
        // Check for saved token
        if let savedToken = UserDefaults.standard.string(forKey: "authToken") {
            token = savedToken
            isAuthenticated = true
        }
    }

    func login(username: String, password: String) {
        guard !username.isEmpty, !password.isEmpty else {
            error = "Username and password are required"
            return
        }

        isLoading = true
        error = nil

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

            URLSession.shared.dataTaskPublisher(for: request)
                .map { $0.data }
                .decode(type: AuthResponse.self, decoder: JSONDecoder())
                .receive(on: DispatchQueue.main)
                .sink(receiveCompletion: { [weak self] completion in
                    self?.isLoading = false

                    if case let .failure(error) = completion {
                        if let urlError = error as? URLError {
                            switch urlError.code {
                            case .notConnectedToInternet:
                                self?.error = "No internet connection"
                            case .cannotConnectToHost:
                                self?.error = "Cannot connect to server"
                            case .timedOut:
                                self?.error = "Connection timed out"
                            default:
                                self?.error = "Network error: \(urlError.localizedDescription)"
                            }
                        } else {
                            self?.error = "Invalid username or password"
                        }
                    }
                }, receiveValue: { [weak self] response in
                    self?.token = response.token
                    self?.isAuthenticated = true

                    // Save token
                    UserDefaults.standard.set(response.token, forKey: "authToken")
                })
                .store(in: &cancellables)
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
