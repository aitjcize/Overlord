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
    @Published var clients: [Client] = []
    @Published var cameras: [String: Camera] = [:]
    private var filterPattern: String = ""
    let apiService: APIService
    private let monitorService: MonitorService

    init() {
        apiService = APIService()
        monitorService = MonitorService()

        // Set up WebSocket event handlers
        monitorService.onAgentJoined = { [weak self] client in
            Task { @MainActor in
                self?.handleAgentJoined(client)
            }
        }
        monitorService.onAgentLeft = { [weak self] client in
            Task { @MainActor in
                self?.handleAgentLeft(client)
            }
        }
    }

    func startMonitoring() {
        monitorService.start()
    }

    func stopMonitoring() {
        monitorService.stop()
    }

    @MainActor
    func loadInitialClients() async {
        guard let token = UserDefaults.standard.string(forKey: "authToken") else { return }

        do {
            let clientsList = try await apiService.getClients(token: token).async()
            // Set the initial clients list first
            clients = clientsList

            // Then load properties for each client
            for client in clientsList {
                await loadClientProperties(client: client, token: token)
            }
        } catch {
            print("Failed to load initial clients: \(error)")
        }
    }

    private func sortClients() {
        clients.sort { client1, client2 in
            if client1.pinned != client2.pinned {
                return client1.pinned && !client2.pinned
            }
            // If both have same pin status, sort by name/mid
            return (client1.name ?? client1.mid) < (client2.name ?? client2.mid)
        }
    }

    private func handleAgentJoined(_ client: Client) {
        // Add or update client in the list, preserving pinned status if it exists
        if let index = clients.firstIndex(where: { $0.mid == client.mid }) {
            let wasPinned = clients[index].pinned
            var updatedClient = client
            updatedClient.pinned = wasPinned
            clients[index] = updatedClient
        } else {
            clients.append(client)
        }

        sortClients()

        // Load client properties
        if let token = UserDefaults.standard.string(forKey: "authToken") {
            Task {
                await loadClientProperties(client: client, token: token)
            }
        }
    }

    private func handleAgentLeft(_ client: Client) {
        clients.removeAll { $0.mid == client.mid }
        sortClients()
    }

    func setFilterPattern(_ pattern: String) {
        filterPattern = pattern
        objectWillChange.send()
    }

    var filteredClients: [Client] {
        if filterPattern.isEmpty {
            return clients // Already sorted when modified
        }
        let filtered = clients.filter { client in
            client.mid.lowercased().contains(filterPattern.lowercased())
        }
        return filtered.sorted { client1, client2 in
            if client1.pinned != client2.pinned {
                return client1.pinned && !client2.pinned
            }
            return (client1.name ?? client1.mid) < (client2.name ?? client2.mid)
        }
    }

    public func loadClientProperties(client: Client, token: String) async {
        do {
            let properties = try await apiService.getClientProperties(mid: client.mid, token: token).async()

            // Update client properties
            if let index = clients.firstIndex(where: { $0.mid == client.mid }) {
                clients[index].properties = properties
            }
        } catch {
            print("Failed to load properties for client \(client.mid): \(error)")
        }
    }

    func togglePinStatus(for mid: String) {
        if let index = clients.firstIndex(where: { $0.mid == mid }) {
            clients[index].pinned.toggle()
            sortClients()
            objectWillChange.send()
        }
    }

    // Camera management functions
    func addCamera(id: String, clientId: String) -> Camera {
        let camera = Camera(id: id, clientId: clientId)
        cameras[id] = camera
        objectWillChange.send()
        return camera
    }

    func removeCamera(id: String) {
        cameras.removeValue(forKey: id)
        objectWillChange.send()
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
