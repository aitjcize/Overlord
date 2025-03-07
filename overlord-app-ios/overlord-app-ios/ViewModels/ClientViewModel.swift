#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import Combine
import CoreGraphics
import Foundation

@MainActor
class ClientViewModel: ObservableObject {
    @Published var clients: [String: Client] = [:]
    @Published var recentClients: [Client] = []
    @Published var cameras: [String: Camera] = [:]
    @Published var activeClientId: String?
    @Published var filterPattern: String = ""

    // Computed property for backward compatibility
    var fixtures: [String: Client] {
        Dictionary(uniqueKeysWithValues: clients.values.filter { $0.pinned }.map { ($0.mid, $0) })
    }

    let apiService: APIService
    let webSocketService: WebSocketService
    private var cancellables = Set<AnyCancellable>()

    var clientsArray: [Client] {
        Array(clients.values)
    }

    var activeRecentClients: [Client] {
        let active = recentClients.filter { clients[$0.mid] != nil }

        // Sort alphabetically by name (case-insensitive)
        return active.sorted {
            let name1 = $0.name?.lowercased() ?? $0.mid.lowercased()
            let name2 = $1.name?.lowercased() ?? $1.mid.lowercased()
            return name1 < name2
        }
    }

    var filteredClients: [Client] {
        let filtered: [Client]
        if filterPattern.isEmpty {
            filtered = clientsArray
        } else {
            let pattern = filterPattern.lowercased()
            filtered = clientsArray
                .filter { $0.mid.lowercased().contains(pattern) || ($0.name?.lowercased().contains(pattern) ?? false) }
        }

        // Sort by pinned status first, then alphabetically by name
        return filtered.sorted {
            if $0.pinned && !$1.pinned {
                return true
            } else if !$0.pinned && $1.pinned {
                return false
            } else {
                let name1 = $0.name?.lowercased() ?? $0.mid.lowercased()
                let name2 = $1.name?.lowercased() ?? $1.mid.lowercased()
                return name1 < name2
            }
        }
    }

    init(apiService: APIService? = nil, webSocketService: WebSocketService? = nil) {
        // Use provided services or create them in a Task
        if let apiService = apiService {
            self.apiService = apiService
        } else {
            // Create a placeholder that will be replaced in the Task below
            self.apiService = APIService()
        }

        if let webSocketService = webSocketService {
            self.webSocketService = webSocketService
        } else {
            // Create a placeholder that will be replaced in the Task below
            self.webSocketService = WebSocketService()
        }

        // Listen for WebSocket authentication failures
        Task { @MainActor in
            NotificationCenter.default.publisher(for: .webSocketAuthenticationFailed)
                .sink { _ in
                    // Handle authentication failure
                    NotificationCenter.default.post(name: .logoutRequested, object: nil)
                }
                .store(in: &cancellables)
        }
    }

    func loadInitialClients(token: String) {
        Task { @MainActor in
            do {
                let clientsList = try await apiService.getClients(token: token).async()

                // Process clients sequentially
                for client in clientsList {
                    await loadClientProperties(client: client, token: token)
                }
            } catch {
                print("Failed to load clients: \(error)")
            }
        }
    }

    private func loadClientProperties(client: Client, token: String) async {
        Task { @MainActor in
            do {
                let properties = try await apiService.getClientProperties(mid: client.mid, token: token).async()

                var updatedClient = client
                updatedClient.properties = properties

                // Add to clients map
                self.clients[client.mid] = updatedClient
            } catch {
                print("Failed to load properties for client \(client.mid): \(error)")
            }
        }
    }

    func setupWebSocketHandlers() {
        webSocketService.on(event: "agent joined") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let client = try? JSONDecoder().decode(Client.self, from: data)
            else {
                return
            }

            Task { @MainActor in
                self.addClient(client)
            }
        }

        webSocketService.on(event: "agent left") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let client = try? JSONDecoder().decode(Client.self, from: data)
            else {
                return
            }

            Task { @MainActor in
                self.removeClient(mid: client.mid)
            }
        }

        webSocketService.on(event: "file download") { [weak self] sid in
            guard let self = self,
                  let token = UserDefaults.standard.string(forKey: "authToken")
            else {
                return
            }

            Task { @MainActor in
                self.apiService.downloadFile(sid: sid, token: token)
            }
        }
    }

    func addClient(_ client: Client) {
        // Preserve pinned status if client already exists
        var newClient = client
        if let existingClient = clients[client.mid] {
            newClient.pinned = existingClient.pinned
        }

        clients[client.mid] = newClient

        // Add to recent clients if connected via WebSocket
        if webSocketService.isConnected {
            // Remove if already exists
            recentClients.removeAll { $0.mid == client.mid }

            // Add to beginning and limit to 5
            recentClients.insert(client, at: 0)
            if recentClients.count > 5 {
                recentClients = Array(recentClients.prefix(5))
            }
        }
    }

    func removeClient(mid: String) {
        clients.removeValue(forKey: mid)
        recentClients.removeAll { $0.mid == mid }
    }

    func togglePinStatus(for mid: String) {
        guard var client = clients[mid] else { return }
        client.pinned.toggle()
        clients[mid] = client
    }

    func addCamera(id: String, clientId: String) -> Camera {
        let camera = Camera(id: id, clientId: clientId)
        cameras[id] = camera
        return camera
    }

    func removeCamera(id: String) {
        cameras.removeValue(forKey: id)
    }

    func setFilterPattern(_ pattern: String) {
        filterPattern = pattern
    }

    func setActiveClientId(_ mid: String?) {
        activeClientId = mid
    }

    static func createClientViewModel() -> ClientViewModel {
        let apiService = APIService()
        let webSocketService = WebSocketService()
        return ClientViewModel(apiService: apiService, webSocketService: webSocketService)
    }

    func addFixture(client: Client) {
        // For backward compatibility
        guard var clientToPin = clients[client.mid] else { return }
        clientToPin.pinned = true
        clients[client.mid] = clientToPin
    }

    func removeFixture(mid: String) {
        // For backward compatibility
        guard var clientToUnpin = clients[mid] else { return }
        clientToUnpin.pinned = false
        clients[mid] = clientToUnpin
    }
}

extension Notification.Name {
    static let logoutRequested = Notification.Name("logoutRequested")
}

// Extension to convert Publisher to async/await
extension Publisher {
    func async() async throws -> Output {
        try await withCheckedThrowingContinuation { continuation in
            var cancellable: AnyCancellable?

            cancellable = self
                .sink(
                    receiveCompletion: { completion in
                        switch completion {
                        case .finished:
                            break
                        case let .failure(error):
                            continuation.resume(throwing: error)
                        }
                        cancellable?.cancel()
                    },
                    receiveValue: { value in
                        continuation.resume(returning: value)
                        cancellable?.cancel()
                    }
                )
        }
    }
}
