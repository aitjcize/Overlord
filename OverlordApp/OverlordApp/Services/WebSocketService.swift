import Foundation
import Combine

class WebSocketService: ObservableObject {
    private var webSocket: URLSessionWebSocketTask?
    private var session: URLSession
    private var isStarted = false
    private var reconnectTimer: Timer?
    private var reconnectAttempts = 0
    private let maxReconnectAttempts = 5
    
    @Published var isConnected = false
    
    private var eventHandlers: [String: [(String) -> Void]] = [:]
    
    init(session: URLSession = .shared) {
        self.session = session
    }
    
    func start(token: String) {
        guard !isStarted else { return }
        
        isStarted = true
        reconnectAttempts = 0
        connect(token: token)
    }
    
    func stop() {
        isStarted = false
        
        reconnectTimer?.invalidate()
        reconnectTimer = nil
        
        webSocket?.cancel(with: .normalClosure, reason: nil)
        webSocket = nil
        
        isConnected = false
    }
    
    private func connect(token: String) {
        guard isStarted, webSocket == nil else { return }
        
        let urlString = "\(APIService.baseURL.replacingOccurrences(of: "http", with: "ws"))/monitor?token=\(token)"
        guard let url = URL(string: urlString) else {
            print("Invalid WebSocket URL")
            return
        }
        
        webSocket = session.webSocketTask(with: url)
        webSocket?.resume()
        
        receiveMessage()
        
        isConnected = true
    }
    
    private func receiveMessage() {
        webSocket?.receive { [weak self] result in
            guard let self = self else { return }
            
            switch result {
            case .success(let message):
                switch message {
                case .string(let text):
                    self.handleMessage(text)
                case .data(let data):
                    if let text = String(data: data, encoding: .utf8) {
                        self.handleMessage(text)
                    }
                @unknown default:
                    break
                }
                
                // Continue receiving messages
                self.receiveMessage()
                
            case .failure(let error):
                print("WebSocket receive error: \(error)")
                self.handleDisconnect()
            }
        }
    }
    
    private func handleMessage(_ text: String) {
        guard let data = text.data(using: .utf8),
              let message = try? JSONDecoder().decode(WebSocketMessage.self, from: data) else {
            return
        }
        
        let handlers = eventHandlers[message.event] ?? []
        let messageData = message.data.first ?? ""
        
        DispatchQueue.main.async {
            handlers.forEach { handler in
                handler(messageData)
            }
        }
    }
    
    private func handleDisconnect() {
        isConnected = false
        webSocket = nil
        
        guard isStarted else { return }
        
        reconnectAttempts += 1
        
        if reconnectAttempts >= maxReconnectAttempts {
            // Too many failed attempts, stop trying
            stop()
            
            // Notify that authentication might have failed
            NotificationCenter.default.post(name: .webSocketAuthenticationFailed, object: nil)
            return
        }
        
        // Try to reconnect after a delay
        reconnectTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: false) { [weak self] _ in
            guard let self = self, let token = UserDefaults.standard.string(forKey: "authToken") else { return }
            self.connect(token: token)
        }
    }
    
    func on(event: String, handler: @escaping (String) -> Void) {
        if eventHandlers[event] == nil {
            eventHandlers[event] = []
        }
        
        eventHandlers[event]?.append(handler)
    }
    
    func off(event: String) {
        eventHandlers[event] = nil
    }
}

struct WebSocketMessage: Codable {
    let event: String
    let data: [String]
}

extension Notification.Name {
    static let webSocketAuthenticationFailed = Notification.Name("webSocketAuthenticationFailed")
} 