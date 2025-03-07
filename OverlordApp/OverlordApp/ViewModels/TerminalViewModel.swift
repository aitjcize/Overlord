import Foundation
import Combine

class TerminalViewModel: ObservableObject {
    @Published var terminals: [String: Terminal] = [:]
    
    private let webSocketService: WebSocketService
    private var cancellables = Set<AnyCancellable>()
    
    var terminalsArray: [Terminal] {
        Array(terminals.values)
    }
    
    init(webSocketService: WebSocketService = WebSocketService()) {
        self.webSocketService = webSocketService
    }
    
    func setupWebSocketHandlers() {
        webSocketService.on(event: "terminal output") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let terminalOutput = try? JSONDecoder().decode(TerminalOutput.self, from: data) else {
                return
            }
            
            self.updateTerminalOutput(terminalOutput)
        }
    }
    
    func createTerminal(for clientId: String, title: String) -> Terminal {
        let terminal = Terminal(clientId: clientId, title: title)
        terminals[terminal.id] = terminal
        
        // Send command to server to create terminal
        sendTerminalCommand(terminalId: terminal.id, clientId: clientId, command: "create")
        
        return terminal
    }
    
    func closeTerminal(id: String) {
        guard let terminal = terminals[id] else { return }
        
        // Send command to server to close terminal
        sendTerminalCommand(terminalId: id, clientId: terminal.clientId, command: "close")
        
        terminals.removeValue(forKey: id)
    }
    
    func sendCommand(terminalId: String, command: String) {
        guard let terminal = terminals[terminalId] else { return }
        
        // Send command to server
        sendTerminalCommand(terminalId: terminalId, clientId: terminal.clientId, command: command)
    }
    
    private func sendTerminalCommand(terminalId: String, clientId: String, command: String) {
        guard let token = UserDefaults.standard.string(forKey: "authToken") else { return }
        
        // In a real app, you would send this to the server
        // This is a placeholder for the actual implementation
        print("Sending terminal command: \(command) to terminal \(terminalId) for client \(clientId)")
    }
    
    private func updateTerminalOutput(_ output: TerminalOutput) {
        DispatchQueue.main.async {
            if var terminal = self.terminals[output.terminalId] {
                terminal.output += output.text
                self.terminals[output.terminalId] = terminal
            }
        }
    }
    
    func minimizeTerminal(id: String) {
        guard var terminal = terminals[id] else { return }
        
        terminal.isMinimized = true
        terminals[id] = terminal
    }
    
    func maximizeTerminal(id: String) {
        guard var terminal = terminals[id] else { return }
        
        terminal.isMinimized = false
        terminals[id] = terminal
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