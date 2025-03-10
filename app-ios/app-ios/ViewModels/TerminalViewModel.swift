import Combine
import CoreGraphics
import Foundation
import SwiftTerm

class TerminalViewModel: ObservableObject {
    @Published var terminals: [String: Terminal] = [:]

    private(set) var webSocketService: WebSocketService
    private var cancellables = Set<AnyCancellable>()
    private var clientTerminalCounts: [String: Int] = [:] // Track terminal counts per client

    var terminalsArray: [Terminal] {
        Array(terminals.values)
    }

    // Check if a client has multiple terminals
    func hasMultipleTerminals(for clientId: String) -> Bool {
        let clientTerminals = terminalsArray.filter { $0.clientId == clientId }
        return clientTerminals.count > 1
    }

    init(webSocketService: WebSocketService = WebSocketService()) {
        self.webSocketService = webSocketService
    }

    func setupWebSocketHandlers() {
        webSocketService.on(event: "terminal output") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let terminalOutput = try? JSONDecoder().decode(TerminalOutput.self, from: data)
            else {
                return
            }

            self.handleTerminalOutput(terminalOutput)
        }
    }

    // Get sequential ID for a client's terminal
    private func getSequentialIdForClient(_ clientId: String) -> Int {
        if clientTerminalCounts[clientId] == nil {
            clientTerminalCounts[clientId] = 1
        } else {
            clientTerminalCounts[clientId]! += 1
        }
        return clientTerminalCounts[clientId]!
    }

    // Create a terminal instance without adding it to the published collection
    func prepareTerminal(for clientId: String, title: String) -> Terminal {
        let sequentialId = getSequentialIdForClient(clientId)
        return Terminal(clientId: clientId, title: title, clientSequentialId: sequentialId)
    }

    // Add a terminal to the published collection and connect its WebSocket
    func addTerminal(_ terminal: Terminal) {
        terminals[terminal.id] = terminal

        // Connect WebSocket for this terminal
        connectWebSocket(for: terminal)

        // Notify observers that a terminal was added
        objectWillChange.send()
    }

    // Original method, now implemented using the two methods above
    func createTerminal(for clientId: String, title: String) -> Terminal {
        let terminal = prepareTerminal(for: clientId, title: title)
        addTerminal(terminal)
        return terminal
    }

    func closeTerminal(id: String) {
        guard let terminal = terminals[id] else { return }

        // Cancel WebSocket connection
        if let webSocket = terminal.webSocket {
            webSocket.cancel(with: .goingAway, reason: nil)
        }

        // Remove terminal from collection
        terminals.removeValue(forKey: id)

        // Check if all terminals for this client are removed, reset the counter
        let clientId = terminal.clientId
        let remainingTerminals = terminalsArray.filter { $0.clientId == clientId }
        if remainingTerminals.isEmpty {
            clientTerminalCounts[clientId] = 0
        }

        // Notify observers that a terminal was removed
        objectWillChange.send()
    }

    func connectWebSocket(for terminal: Terminal) {
        guard let token = UserDefaults.standard.string(forKey: "authToken"),
              let serverAddress = UserDefaults.standard.string(forKey: "serverAddress")
        else {
            return
        }

        // Create WebSocket URL
        let wsProtocol = serverAddress.hasPrefix("https") ? "wss://" : "ws://"
        let baseAddress = serverAddress.replacingOccurrences(of: "https://", with: "")
            .replacingOccurrences(of: "http://", with: "")
        let wsURL = URL(string: "\(wsProtocol)\(baseAddress)/api/agents/\(terminal.clientId)/tty?token=\(token)")!

        // Create WebSocket task
        let session = URLSession(configuration: .default)
        let webSocket = session.webSocketTask(with: wsURL)

        // Store WebSocket in terminal on main thread
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }
            if var updatedTerminal = self.terminals[terminal.id] {
                updatedTerminal.webSocket = webSocket
                self.terminals[terminal.id] = updatedTerminal
            }
        }

        // Handle incoming messages
        receiveWebSocketMessages(webSocket, terminalId: terminal.id)

        // Connect
        webSocket.resume()
    }

    private func receiveWebSocketMessages(_ webSocket: URLSessionWebSocketTask, terminalId: String) {
        webSocket.receive { [weak self] result in
            guard let self = self else { return }

            DispatchQueue.main.async {
                switch result {
                case let .success(message):
                    self.handleWebSocketMessage(message, terminalId: terminalId)

                    // Continue receiving messages only if the terminal still exists
                    if self.terminals[terminalId] != nil {
                        self.receiveWebSocketMessages(webSocket, terminalId: terminalId)
                    }

                case let .failure(error):
                    print("WebSocket receive error: \(error)")

                    // Check if this is an authentication error (401)
                    if let nsError = error as NSError?,
                       nsError.domain == NSURLErrorDomain,
                       nsError.code == NSURLErrorUserAuthenticationRequired
                    {
                        // Post notification to trigger logout
                        NotificationCenter.default.post(name: .logoutRequested, object: nil)
                        return
                    }

                    // Try to reconnect after a delay, but only if the terminal still exists
                    DispatchQueue.main.asyncAfter(deadline: .now() + 1) {
                        if let terminal = self.terminals[terminalId] {
                            self.connectWebSocket(for: terminal)
                        }
                    }
                }
            }
        }
    }

    // Helper method to handle WebSocket messages
    private func handleWebSocketMessage(_ message: URLSessionWebSocketTask.Message, terminalId: String) {
        switch message {
        case let .data(data):
            // Handle binary data
            handleTerminalData(data, terminalId: terminalId)
        case let .string(text):
            // Handle text data
            handleTextMessage(text, terminalId: terminalId)
        @unknown default:
            break
        }
    }

    // Helper method to handle text messages
    private func handleTextMessage(_ text: String, terminalId: String) {
        // Check if it's a JSON message with session ID
        if let data = text.data(using: .utf8) {
            do {
                // Try to parse as JSON
                if let json = try JSONSerialization.jsonObject(with: data, options: []) as? [String: Any],
                   let type = json["type"] as? String, type == "sid",
                   let sid = json["data"] as? String
                {
                    // This is a session ID message, store it in the terminal
                    if var terminal = terminals[terminalId] {
                        terminal.sid = sid
                        terminals[terminalId] = terminal
                    }
                } else {
                    // Not a session ID message, send to terminal
                    handleTerminalData(data, terminalId: terminalId)
                }
            } catch {
                // Not valid JSON, treat as regular terminal output
                handleTerminalData(data, terminalId: terminalId)
            }
        }
    }

    func sendData(terminalId: String, data: [UInt8]) {
        DispatchQueue.main.async { [weak self] in
            guard let self = self,
                  let terminal = self.terminals[terminalId],
                  let webSocket = terminal.webSocket
            else {
                return
            }

            // Send data as binary message
            let data = Data(data)
            webSocket.send(.data(data)) { error in
                if let error = error {
                    print("Error sending data: \(error)")
                }
            }
        }
    }

    private func handleTerminalData(_ data: Data, terminalId: String) {
        DispatchQueue.main.async {
            // Forward data to terminal view through notification
            NotificationCenter.default.post(
                name: .init("TerminalDataReceived"),
                object: nil,
                userInfo: [
                    "terminalId": terminalId,
                    "data": data
                ]
            )
        }
    }

    private func handleTerminalOutput(_ output: TerminalOutput) {
        guard let data = output.text.data(using: .utf8) else { return }
        handleTerminalData(data, terminalId: output.terminalId)
    }

    func minimizeTerminal(id: String) {
        DispatchQueue.main.async { [weak self] in
            guard let self = self,
                  var terminal = self.terminals[id] else { return }

            terminal.isMinimized = true
            self.terminals[id] = terminal

            // Notify observers that the terminal has been minimized
            self.objectWillChange.send()
        }
    }

    func maximizeTerminal(id: String) {
        DispatchQueue.main.async { [weak self] in
            guard let self = self,
                  var terminal = self.terminals[id] else { return }

            terminal.isMinimized = false
            self.terminals[id] = terminal

            // Notify observers that the terminal has been maximized
            self.objectWillChange.send()
        }
    }

    func updateTerminalPosition(id: String, position: CGPoint) {
        guard var terminal = terminals[id] else { return }

        terminal.position = position
        terminals[id] = terminal
    }

    func updateTerminalSize(id: String, size: CGSize) {
        guard var terminal = terminals[id] else { return }

        terminal.size = size
        terminals[id] = terminal
    }
}

struct TerminalOutput: Codable {
    let terminalId: String
    let text: String
}
