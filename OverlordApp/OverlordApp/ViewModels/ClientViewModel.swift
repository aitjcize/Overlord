import Foundation
import Combine

class ClientViewModel: ObservableObject {
    @Published var clients: [String: Client] = [:]
    @Published var recentClients: [Client] = []
    @Published var fixtures: [String: Client] = [:]
    @Published var cameras: [String: Camera] = [:]
    @Published var activeClientId: String?
    @Published var filterPattern: String = ""
    
    private let apiService: APIService
    private let webSocketService: WebSocketService
    private var cancellables = Set<AnyCancellable>()
    
    var clientsArray: [Client] {
        Array(clients.values)
    }
    
    var activeRecentClients: [Client] {
        recentClients.filter { clients[$0.mid] != nil }
    }
    
    var filteredClients: [Client] {
        if filterPattern.isEmpty {
            return clientsArray
        }
        
        let pattern = filterPattern.lowercased()
        return clientsArray.filter { $0.mid.lowercased().contains(pattern) }
    }
    
    init(apiService: APIService = APIService(), webSocketService: WebSocketService = WebSocketService()) {
        self.apiService = apiService
        self.webSocketService = webSocketService
        
        // Listen for WebSocket authentication failures
        NotificationCenter.default.publisher(for: .webSocketAuthenticationFailed)
            .sink { [weak self] _ in
                // Handle authentication failure
                NotificationCenter.default.post(name: .logoutRequested, object: nil)
            }
            .store(in: &cancellables)
    }
    
    func loadInitialClients(token: String) {
        apiService.getClients(token: token)
            .receive(on: DispatchQueue.main)
            .sink(receiveCompletion: { completion in
                if case .failure(let error) = completion {
                    print("Failed to load clients: \(error)")
                }
            }, receiveValue: { [weak self] clientsList in
                guard let self = self else { return }
                
                // Process clients sequentially
                for client in clientsList {
                    self.loadClientProperties(client: client, token: token)
                }
            })
            .store(in: &cancellables)
    }
    
    private func loadClientProperties(client: Client, token: String) {
        apiService.getClientProperties(mid: client.mid, token: token)
            .receive(on: DispatchQueue.main)
            .sink(receiveCompletion: { completion in
                if case .failure(let error) = completion {
                    print("Failed to load properties for client \(client.mid): \(error)")
                }
            }, receiveValue: { [weak self] properties in
                guard let self = self else { return }
                
                var updatedClient = client
                updatedClient.properties = properties
                
                // Add to clients map
                self.clients[client.mid] = updatedClient
            })
            .store(in: &cancellables)
    }
    
    func setupWebSocketHandlers() {
        webSocketService.on(event: "agent joined") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let client = try? JSONDecoder().decode(Client.self, from: data) else {
                return
            }
            
            self.addClient(client)
        }
        
        webSocketService.on(event: "agent left") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let client = try? JSONDecoder().decode(Client.self, from: data) else {
                return
            }
            
            self.removeClient(mid: client.mid)
        }
        
        webSocketService.on(event: "file download") { [weak self] sid in
            guard let self = self,
                  let token = UserDefaults.standard.string(forKey: "authToken") else {
                return
            }
            
            self.apiService.downloadFile(sid: sid, token: token)
        }
    }
    
    func addClient(_ client: Client) {
        clients[client.mid] = client
        
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
        removeFixture(mid: mid)
    }
    
    func addFixture(client: Client) {
        guard fixtures[client.mid] == nil else { return }
        
        fixtures[client.mid] = client
        
        // Limit to 8 fixtures
        if fixtures.count > 8 {
            fixtures.removeValue(forKey: fixtures.keys.first!)
        }
    }
    
    func removeFixture(mid: String) {
        fixtures.removeValue(forKey: mid)
    }
    
    func addCamera(id: String, clientId: String) {
        cameras[id] = Camera(id: id, clientId: clientId)
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
}

extension Notification.Name {
    static let logoutRequested = Notification.Name("logoutRequested")
} 