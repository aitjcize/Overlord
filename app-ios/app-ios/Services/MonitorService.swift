import Combine
import Foundation

@MainActor
class MonitorService: ObservableObject {
    static var baseURL: String {
        if let savedAddress = UserDefaults.standard.string(forKey: "serverAddress") {
            // Convert http:// to ws:// or https:// to wss://
            if savedAddress.hasPrefix("https://") {
                return savedAddress.replacingOccurrences(of: "https://", with: "wss://")
            } else {
                return savedAddress.replacingOccurrences(of: "http://", with: "ws://")
            }
        }
        return "ws://localhost:8080"
    }

    static var restBaseURL: String {
        if let savedAddress = UserDefaults.standard.string(forKey: "serverAddress") {
            return savedAddress
        }
        return "http://localhost:8080"
    }

    private var webSocket: URLSessionWebSocketTask?
    private var isStarted = false
    private var reconnectTimer: Timer?
    private var reconnectAttempts = 0
    private let maxReconnectAttempts = 5
    private var cancellables = Set<AnyCancellable>()

    @Published var isConnected = false

    var onAgentJoined: ((Client) -> Void)?
    var onAgentLeft: ((Client) -> Void)?

    init() {
        // Listen for server address changes
        NotificationCenter.default.publisher(for: UserDefaults.didChangeNotification)
            .sink { [weak self] _ in
                Task { @MainActor [weak self] in
                    if self?.isConnected == true {
                        self?.disconnect()
                        if let token = UserDefaults.standard.string(forKey: "authToken") {
                            self?.connect(token: token)
                        }
                    }
                }
            }
            .store(in: &cancellables)
    }

    func start() {
        guard !isStarted else { return }

        if let token = UserDefaults.standard.string(forKey: "authToken") {
            isStarted = true
            reconnectAttempts = 0
            connect(token: token)
        } else {
            print("Monitor service not started: User not authenticated")
        }
    }

    func stop() {
        isStarted = false

        reconnectTimer?.invalidate()
        reconnectTimer = nil

        webSocket?.cancel()
        webSocket = nil
        isConnected = false
    }

    private func connect(token: String) {
        guard isStarted, webSocket == nil else { return }

        // Create WebSocket URL with token
        guard let url = URL(string: "\(MonitorService.baseURL)/api/monitor?token=\(token)") else {
            print("Failed to create WebSocket URL")
            return
        }

        print("Connecting to WebSocket at \(url)")
        let session = URLSession(configuration: .default)
        webSocket = session.webSocketTask(with: url)

        webSocket?.resume()
        isConnected = true
        receiveMessage()
    }

    private func receiveMessage() {
        webSocket?.receive { [weak self] result in
            switch result {
            case let .success(message):
                switch message {
                case let .string(text):
                    Task { @MainActor [weak self] in
                        self?.handleMessage(text)
                    }
                case let .data(data):
                    if let text = String(data: data, encoding: .utf8) {
                        Task { @MainActor [weak self] in
                            self?.handleMessage(text)
                        }
                    }
                @unknown default:
                    break
                }
                // Continue receiving messages
                Task { @MainActor [weak self] in
                    self?.receiveMessage()
                }

            case let .failure(error):
                print("WebSocket receive error: \(error)")
                Task { @MainActor [weak self] in
                    self?.handleDisconnection()
                }
            }
        }
    }

    private func handleMessage(_ text: String) {
        guard let data = text.data(using: .utf8),
              let message = try? JSONDecoder().decode(WebSocketMessage.self, from: data)
        else {
            return
        }

        switch message.event {
        case "agent joined":
            if let clientData = message.data.first?.data(using: .utf8),
               let client = try? JSONDecoder().decode(Client.self, from: clientData)
            {
                onAgentJoined?(client)
            }
        case "agent left":
            if let clientData = message.data.first?.data(using: .utf8),
               let client = try? JSONDecoder().decode(Client.self, from: clientData)
            {
                onAgentLeft?(client)
            }
        default:
            break
        }
    }

    private func handleDisconnection() {
        webSocket = nil
        isConnected = false

        guard isStarted else { return }

        reconnectAttempts += 1
        print("Reconnect attempt \(reconnectAttempts) of \(maxReconnectAttempts)")

        if reconnectAttempts >= maxReconnectAttempts {
            print("WebSocket authentication failed after multiple attempts")
            // Handle authentication failure (e.g., log out user)
            NotificationCenter.default.post(name: NSNotification.Name("LogoutUser"), object: nil)
            return
        }

        // Reconnect after 1 second
        reconnectTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: false) { [weak self] _ in
            if let token = UserDefaults.standard.string(forKey: "authToken") {
                Task { @MainActor [weak self] in
                    self?.connect(token: token)
                }
            }
        }
    }

    func disconnect() {
        webSocket?.cancel(with: .normalClosure, reason: nil)
        webSocket = nil
        isConnected = false
    }
}
