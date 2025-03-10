import Combine
import Foundation
import Network
import ObjectiveC

class PortForwardViewModel: ObservableObject {
    @Published var portForwards: [String: PortForward] = [:]
    private var webSocketService: WebSocketService
    private var cancellables = Set<AnyCancellable>()
    private var localPortCounter: Int = 8000 // Start local ports from 8000

    // Store TCP listeners and connections
    private var listeners: [String: NWListener] = [:]
    private var connections: [String: [NWConnection]] = [:]

    // Add a serial queue for thread-safe access to connection pairs
    private let connectionQueue = DispatchQueue(label: "com.app.connectionQueue")

    // Track network permission status
    @Published var networkPermissionGranted: Bool = true

    // Track the most recently created port forward and whether to show it
    @Published var lastCreatedPortForward: PortForward?
    @Published var shouldShowPortForwardWebView: Bool = false

    // Create a connection pair class to track TCP and WebSocket pairs
    private class ConnectionPair {
        let id = UUID().uuidString
        let portForwardId: String
        let tcpConnection: NWConnection
        var webSocket: URLSessionWebSocketTask?
        var isActive = true

        init(portForwardId: String, tcpConnection: NWConnection) {
            self.portForwardId = portForwardId
            self.tcpConnection = tcpConnection
        }

        func close() {
            isActive = false
            tcpConnection.cancel()
            webSocket?.cancel(with: .goingAway, reason: nil)
        }
    }

    // In PortForwardViewModel, add a dictionary to track connection pairs
    private var connectionPairs: [String: ConnectionPair] = [:]

    var portForwardsArray: [PortForward] {
        Array(portForwards.values)
    }

    init(webSocketService: WebSocketService = WebSocketService()) {
        self.webSocketService = webSocketService
    }

    // Helper method for logging errors
    private func logError(_ message: String, error: Error? = nil) {
        if let error = error {
            print("[PortForward Error] \(message): \(error)")
        } else {
            print("[PortForward Error] \(message)")
        }
    }

    func createPortForward(
        for client: Client,
        remoteHost: String,
        remotePort: Int,
        useHttps: Bool = false
    ) -> PortForward {
        // Increment local port counter to get a unique local port
        localPortCounter += 1
        let localPort = localPortCounter

        let portForward = PortForward(
            clientId: client.mid,
            clientName: client.name ?? client.mid,
            remoteHost: remoteHost,
            remotePort: remotePort,
            localPort: localPort,
            useHttps: useHttps
        )

        // Store the port forward
        portForwards[portForward.id] = portForward

        // Start local TCP listener
        startLocalServer(for: portForward)

        // Set as the last created port forward
        lastCreatedPortForward = portForward

        // Add a small delay to ensure the port forward is ready before showing the WebView
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in
            self?.shouldShowPortForwardWebView = true
        }

        // Notify observers that a port forward was added
        objectWillChange.send()

        return portForward
    }

    func closePortForward(id: String) {
        guard portForwards[id] != nil else { return }

        // Close all connection pairs for this port forward - use the connection queue
        connectionQueue.sync {
            for (_, connectionPair) in connectionPairs where connectionPair.portForwardId == id {
                connectionPair.close()
            }
            connectionPairs = connectionPairs.filter { $0.value.portForwardId != id }

            // Close all active connections (for backward compatibility)
            if let activeConnections = connections[id] {
                for connection in activeConnections {
                    connection.cancel()
                }
                connections.removeValue(forKey: id)
            }
        }

        // Stop TCP listener
        if let listener = listeners[id] {
            listener.cancel()
            listeners.removeValue(forKey: id)
        }

        // Remove port forward from collection
        portForwards.removeValue(forKey: id)

        // If this was the last created port forward, clear it
        if lastCreatedPortForward?.id == id {
            lastCreatedPortForward = nil
            shouldShowPortForwardWebView = false
        }

        // Notify observers that a port forward was removed
        objectWillChange.send()
    }

    private func startLocalServer(for portForward: PortForward) {
        // Create a TCP listener on localhost with the specified port
        let port = NWEndpoint.Port(integerLiteral: UInt16(portForward.localPort))
        let parameters = configureTCPParameters()

        do {
            let listener = try NWListener(using: parameters, on: port)

            // Set up listener state handler
            configureListenerStateHandler(listener, for: portForward)

            // Set up new connection handler
            listener.newConnectionHandler = { [weak self] connection in
                guard let self = self else { return }
                self.handleNewConnection(connection, portForwardId: portForward.id)
            }

            // Start the listener
            listener.start(queue: .global())

            // Store the listener
            listeners[portForward.id] = listener
        } catch {
            // Handle the error
            handleListenerCreationError(error, for: portForward)
        }
    }

    private func handleNewConnection(_ connection: NWConnection, portForwardId: String) {
        guard let portForward = portForwards[portForwardId] else {
            connection.cancel()
            return
        }

        // Create a new connection pair
        let connectionPair = ConnectionPair(portForwardId: portForwardId, tcpConnection: connection)

        // Store the connection pair - use the connection queue for thread safety
        connectionQueue.sync {
            connectionPairs[connectionPair.id] = connectionPair

            // Also track in the connections dictionary for backward compatibility
            // Use the same queue for thread safety
            if connections[portForwardId] == nil {
                connections[portForwardId] = []
            }
            connections[portForwardId]?.append(connection)
        }

        // Set up connection state handler
        connection.stateUpdateHandler = { [weak self] state in
            guard let self = self else { return }
            switch state {
            case .ready:
                // Create a new WebSocket connection for this TCP connection
                self.createWebSocketForConnectionPair(connectionPair, portForward: portForward)
                // Start receiving data from the connection
                self.receiveData(from: connectionPair)
            case let .failed(error):
                self.logError("TCP connection failed", error: error)
                self.removeConnectionPair(connectionPair)
            case .cancelled:
                self.removeConnectionPair(connectionPair)
            case .preparing, .setup, .waiting:
                break
            @unknown default:
                break
            }
        }

        // Start the connection
        connection.start(queue: .global())
    }

    private func createWebSocketForConnectionPair(_ connectionPair: ConnectionPair, portForward: PortForward) {
        guard let token = UserDefaults.standard.string(forKey: "authToken"),
              let serverAddress = UserDefaults.standard.string(forKey: "serverAddress")
        else {
            logError("Missing auth token or server address")
            return
        }

        // Create WebSocket URL
        let wsProtocol = serverAddress.hasPrefix("https") ? "wss://" : "ws://"
        let baseAddress = serverAddress.replacingOccurrences(of: "https://", with: "")
            .replacingOccurrences(of: "http://", with: "")
        let urlString = "\(wsProtocol)\(baseAddress)/api/agents/\(portForward.clientId)/forward" +
            "?host=\(portForward.remoteHost)" +
            "&port=\(portForward.remotePort)" +
            "&token=\(token)"
        let wsURL = URL(string: urlString)!

        // Create WebSocket task
        let session = URLSession(configuration: .default)
        let webSocket = session.webSocketTask(with: wsURL)

        // Store WebSocket in connection pair
        connectionPair.webSocket = webSocket

        // Connect
        webSocket.resume()

        // Set up WebSocket receive handler
        receiveWebSocketMessage(connectionPair)
    }

    private func receiveData(from connectionPair: ConnectionPair) {
        connectionPair.tcpConnection
            .receive(minimumIncompleteLength: 1, maximumLength: 65536) { [weak self] data, _, isComplete, error in
                guard let self = self, connectionPair.isActive else { return }

                if let error = error {
                    self.logError("Error receiving data from TCP connection", error: error)
                    self.removeConnectionPair(connectionPair)
                    return
                }

                if let data = data, !data.isEmpty {
                    // Forward data to WebSocket
                    self.sendDataToWebSocket(data, connectionPair: connectionPair)
                }

                if isComplete {
                    // Connection closed
                    self.removeConnectionPair(connectionPair)
                    return
                }

                // Continue receiving data
                self.receiveData(from: connectionPair)
            }
    }

    private func sendDataToWebSocket(_ data: Data, connectionPair: ConnectionPair) {
        guard let webSocket = connectionPair.webSocket else {
            return
        }

        // Send data as binary message
        webSocket.send(.data(data)) { [weak self] error in
            if let error = error {
                self?.logError("Error sending data to WebSocket", error: error)
                self?.removeConnectionPair(connectionPair)
            }
        }
    }

    private func receiveWebSocketMessage(_ connectionPair: ConnectionPair) {
        guard let webSocket = connectionPair.webSocket, connectionPair.isActive else {
            return
        }

        webSocket.receive { [weak self] result in
            guard let self = self, connectionPair.isActive else { return }

            switch result {
            case let .success(message):
                // Process the message
                switch message {
                case let .data(data):
                    // Forward data to TCP connection
                    self.sendDataToTCPConnection(data, connectionPair: connectionPair)
                case let .string(text):
                    // Convert string to data and forward
                    if let data = text.data(using: .utf8) {
                        self.sendDataToTCPConnection(data, connectionPair: connectionPair)
                    }
                @unknown default:
                    break
                }

                // Continue receiving messages
                if connectionPair.isActive {
                    self.receiveWebSocketMessage(connectionPair)
                }

            case let .failure(error):
                // Close the connection pair on error
                self.logError("WebSocket receive error", error: error)

                // Check if this is an authentication error (401)
                if let nsError = error as NSError?,
                   nsError.domain == NSURLErrorDomain,
                   nsError.code == NSURLErrorUserAuthenticationRequired
                {
                    // Post notification to trigger logout
                    NotificationCenter.default.post(name: .logoutRequested, object: nil)
                    return
                }

                self.removeConnectionPair(connectionPair)
            }
        }
    }

    private func sendDataToTCPConnection(_ data: Data, connectionPair: ConnectionPair) {
        connectionPair.tcpConnection.send(content: data, completion: .contentProcessed { [weak self] error in
            guard let self = self else { return }

            if let error = error {
                self.logError("Error sending data to TCP connection", error: error)
                self.removeConnectionPair(connectionPair)
            }
        })
    }

    private func removeConnectionPair(_ connectionPair: ConnectionPair) {
        guard connectionPair.isActive else { return }

        connectionPair.close()

        // Use the connection queue for thread safety
        connectionQueue.sync {
            connectionPairs.removeValue(forKey: connectionPair.id)

            // Also remove from the connections dictionary for backward compatibility
            if var activeConnections = connections[connectionPair.portForwardId] {
                activeConnections.removeAll { $0 === connectionPair.tcpConnection }
                connections[connectionPair.portForwardId] = activeConnections
            }
        }
    }
}

// MARK: - PortForwardViewModel TCP Listener Helpers

extension PortForwardViewModel {
    // Helper method to configure TCP parameters
    private func configureTCPParameters() -> NWParameters {
        let parameters = NWParameters.tcp

        // Set SO_REUSEADDR socket option to allow reusing the port
        if let tcpOptions = parameters.defaultProtocolStack.internetProtocol as? NWProtocolTCP.Options {
            tcpOptions.enableKeepalive = true
            tcpOptions.keepaliveIdle = 60
        }

        return parameters
    }

    // Helper method to configure the listener state handler
    private func configureListenerStateHandler(_ listener: NWListener, for portForward: PortForward) {
        listener.stateUpdateHandler = { [weak self] state in
            guard let self = self else { return }

            switch state {
            case .ready:
                self.handleListenerReady(for: portForward)
            case let .failed(error):
                self.handleListenerFailure(error, for: portForward)
            case .cancelled, .setup, .waiting:
                break
            @unknown default:
                break
            }
        }
    }

    // Helper method to handle listener ready state
    private func handleListenerReady(for portForward: PortForward) {
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }

            if let updatedPortForward = self.portForwards[portForward.id] {
                var modifiedPortForward = updatedPortForward
                modifiedPortForward.isActive = true
                self.portForwards[portForward.id] = modifiedPortForward
                self.objectWillChange.send()
            }
        }
    }

    // Helper method to handle listener failure
    private func handleListenerFailure(_ error: Error, for portForward: PortForward) {
        if isAddressInUseError(error) {
            handleAddressInUseError(for: portForward)
        } else {
            // Other errors - just close the port forward
            logError("TCP listener failed", error: error)
            DispatchQueue.main.async { [weak self] in
                self?.closePortForward(id: portForward.id)
            }
        }
    }

    // Helper method to check if error is "address already in use"
    private func isAddressInUseError(_ error: Error) -> Bool {
        let nsError = error as NSError
        return nsError.domain == "NSPOSIXErrorDomain" && nsError.code == 48
    }

    // Helper method to handle "address already in use" error
    private func handleAddressInUseError(for portForward: PortForward) {
        DispatchQueue.main.async { [weak self] in
            guard let self = self,
                  let updatedPortForward = self.portForwards[portForward.id] else { return }

            self.closePortForward(id: portForward.id)

            // Create a new port forward with a different port
            _ = self.createPortForward(
                for: Client(mid: updatedPortForward.clientId, name: updatedPortForward.clientName),
                remoteHost: updatedPortForward.remoteHost,
                remotePort: updatedPortForward.remotePort,
                useHttps: updatedPortForward.useHttps
            )
        }
    }

    // Helper method to handle listener creation error
    private func handleListenerCreationError(_ error: Error, for portForward: PortForward) {
        logError("Failed to create TCP listener", error: error)

        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }

            // Update the port forward status
            if let updatedPortForward = self.portForwards[portForward.id] {
                var modifiedPortForward = updatedPortForward
                modifiedPortForward.isActive = false
                self.portForwards[portForward.id] = modifiedPortForward
                self.objectWillChange.send()
            }
        }
    }

    // Public method to restart the TCP server for a specific port forward
    // This is used when a user taps on a port forward after the app has been in the background
    func restartTCPServerIfNeeded(for portForwardId: String) {
        guard let portForward = portForwards[portForwardId] else { return }

        // Check if the listener exists and is active
        if let listener = listeners[portForwardId], listener.state == .ready {
            // Listener is already active, no need to restart
            return
        }

        // If the listener doesn't exist or is not active, restart it
        if listeners[portForwardId] != nil {
            // Cancel the existing listener first
            listeners[portForwardId]?.cancel()
            listeners.removeValue(forKey: portForwardId)
        }

        // Start a new TCP listener
        startLocalServer(for: portForward)

        // Log the restart
        print("[PortForward] Restarting TCP server for port forward \(portForwardId)")
    }

    // Public method to restart all TCP servers
    // This is used when the app comes back from the background
    func restartAllTCPServers() {
        print("[PortForward] Restarting all TCP servers")

        // Iterate through all port forwards and restart their TCP servers if needed
        for (portForwardId, portForward) in portForwards {
            // Check if the listener exists and is active
            if let listener = listeners[portForwardId], listener.state == .ready {
                // Listener is already active, no need to restart
                continue
            }

            // If the listener doesn't exist or is not active, restart it
            if listeners[portForwardId] != nil {
                // Cancel the existing listener first
                listeners[portForwardId]?.cancel()
                listeners.removeValue(forKey: portForwardId)
            }

            // Start a new TCP listener
            startLocalServer(for: portForward)

            // Log the restart
            print("[PortForward] Restarted TCP server for port forward \(portForwardId)")
        }
    }
}

// Associated objects key for tracking WebSocket handlers
private enum AssociatedKeys {
    static var hasReceiveHandler = "hasReceiveHandler"
}
