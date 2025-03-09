#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import SwiftUI

struct TerminalsView: View {
    @ObservedObject var terminalViewModel: TerminalViewModel
    @State private var showingTerminal = false
    @State private var selectedTerminal: Terminal?

    var body: some View {
        ZStack {
            Color(hex: "0f172a").ignoresSafeArea()

            if terminalViewModel.terminalsArray.isEmpty {
                VStack(spacing: 20) {
                    Image(systemName: "terminal")
                        .font(.system(size: 60))
                        .foregroundColor(Color(hex: "64748b"))

                    Text("No Active Terminals")
                        .font(.title2)
                        .fontWeight(.semibold)
                        .foregroundColor(.white)

                    Text("Open a terminal by tapping the terminal button on a client")
                        .font(.body)
                        .foregroundColor(Color(hex: "94a3b8"))
                        .multilineTextAlignment(.center)
                        .padding(.horizontal)
                }
            } else {
                List {
                    ForEach(terminalViewModel.terminalsArray) { terminal in
                        TerminalRow(terminal: terminal, terminalViewModel: terminalViewModel)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                selectedTerminal = terminal
                                terminalViewModel.maximizeTerminal(id: terminal.id)
                                showingTerminal = true
                            }
                    }
                    .onDelete { indexSet in
                        for index in indexSet {
                            let terminal = terminalViewModel.terminalsArray[index]
                            terminalViewModel.closeTerminal(id: terminal.id)
                        }
                    }
                }
                .listStyle(PlainListStyle())
            }
        }
        .sheet(isPresented: $showingTerminal) {
            if let terminal = selectedTerminal {
                TerminalView(terminal: terminal, terminalViewModel: terminalViewModel)
            }
        }
        .onAppear {
            // Debug: Print all terminals when view appears
            print("Terminals in ViewModel: \(terminalViewModel.terminalsArray.count)")
            for terminal in terminalViewModel.terminalsArray {
                print("Terminal ID: \(terminal.id), Title: \(terminal.title), Minimized: \(terminal.isMinimized)")
            }
        }
    }
}

struct TerminalRow: View {
    let terminal: Terminal
    @ObservedObject var terminalViewModel: TerminalViewModel

    var body: some View {
        HStack {
            Image(systemName: "terminal")
                .font(.system(size: 20))
                .foregroundColor(Color(hex: "10b981"))
                .frame(width: 40, height: 40)
                .background(Color(hex: "334155"))
                .cornerRadius(8)

            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Text(terminal.title)
                        .font(.headline)
                        .foregroundColor(.white)

                    if terminalViewModel.hasMultipleTerminals(for: terminal.clientId) {
                        Text("\(terminal.clientSequentialId)")
                            .font(.caption)
                            .fontWeight(.bold)
                            .foregroundColor(.white)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color(hex: "3b82f6"))
                            .cornerRadius(10)
                    }
                }

                Text(terminal.sid != nil ? "Session ID: \(terminal.sid!)" : "No Session ID")
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
            }
        }
        .padding(.vertical, 8)
        .listRowBackground(Color(hex: "1e293b"))
    }
}

struct TerminalsView_Previews: PreviewProvider {
    static var previews: some View {
        let viewModel = TerminalViewModel()
        // Add some sample terminals for preview
        _ = viewModel.terminals["1"] = Terminal(id: "1", clientId: "client1", title: "Terminal 1")
        _ = viewModel.terminals["2"] = Terminal(id: "2", clientId: "client2", title: "Terminal 2")

        return TerminalsView(terminalViewModel: viewModel)
    }
}
