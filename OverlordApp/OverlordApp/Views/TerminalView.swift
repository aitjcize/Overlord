import SwiftUI

struct TerminalView: View {
    let terminal: Terminal
    @ObservedObject var terminalViewModel: TerminalViewModel
    @State private var commandText = ""
    @Environment(\.presentationMode) var presentationMode
    
    var body: some View {
        NavigationView {
            ZStack {
                Color(hex: "0f172a").ignoresSafeArea()
                
                VStack(spacing: 0) {
                    // Terminal output
                    ScrollView {
                        VStack(alignment: .leading, spacing: 0) {
                            Text(terminal.output)
                                .font(.system(.body, design: .monospaced))
                                .foregroundColor(.white)
                                .padding()
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .background(Color(hex: "1e293b"))
                    
                    // Command input
                    HStack {
                        TextField("Enter command", text: $commandText)
                            .foregroundColor(.white)
                            .padding()
                            .background(Color(hex: "334155"))
                            .cornerRadius(8)
                        
                        Button(action: {
                            sendCommand()
                        }) {
                            Image(systemName: "arrow.up.circle.fill")
                                .font(.system(size: 24))
                                .foregroundColor(Color(hex: "10b981"))
                        }
                        .disabled(commandText.isEmpty)
                        .padding(.horizontal, 8)
                    }
                    .padding()
                    .background(Color(hex: "1e293b"))
                }
            }
            .navigationBarTitle(terminal.title, displayMode: .inline)
            .navigationBarItems(
                trailing: Button(action: {
                    terminalViewModel.closeTerminal(id: terminal.id)
                    presentationMode.wrappedValue.dismiss()
                }) {
                    Text("Close")
                        .foregroundColor(Color(hex: "10b981"))
                }
            )
        }
    }
    
    private func sendCommand() {
        guard !commandText.isEmpty else { return }
        
        terminalViewModel.sendCommand(terminalId: terminal.id, command: commandText)
        
        // Append the command to the output with a prompt
        if var updatedTerminal = terminalViewModel.terminals[terminal.id] {
            updatedTerminal.output += "\n$ \(commandText)\n"
            terminalViewModel.terminals[terminal.id] = updatedTerminal
        }
        
        commandText = ""
    }
}

struct TerminalView_Previews: PreviewProvider {
    static var previews: some View {
        let terminalViewModel = TerminalViewModel()
        let terminal = Terminal(clientId: "client123", title: "Test Terminal")
        
        return TerminalView(terminal: terminal, terminalViewModel: terminalViewModel)
    }
} 