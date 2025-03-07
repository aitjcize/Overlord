#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import SwiftTerm
import SwiftUI

struct TerminalView: View {
    let terminal: Terminal // Our model's Terminal
    @ObservedObject var terminalViewModel: TerminalViewModel
    @Environment(\.presentationMode) var presentationMode
    @State private var isBeingHidden = false

    var body: some View {
        NavigationView {
            ZStack {
                Color(hex: "0f172a").ignoresSafeArea()

                VStack(spacing: 0) {
                    // Terminal view
                    GeometryReader { geometry in
                        TerminalEmulatorView(
                            terminal: terminal,
                            terminalViewModel: terminalViewModel,
                            size: geometry.size
                        )
                        .background(Color(hex: "1e293b"))
                    }
                }
            }
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .principal) {
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
                }
            }
            .toolbarColorScheme(.dark, for: .navigationBar)
            .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
            .toolbarBackground(.visible, for: .navigationBar)
            .navigationBarItems(
                leading: Button(action: {
                    hideTerminal()
                }, label: {
                    Text("Hide")
                        .foregroundColor(Color(hex: "3b82f6"))
                }),
                trailing: Button(action: {
                    closeAndDismiss()
                }, label: {
                    Text("Close")
                        .foregroundColor(Color(hex: "10b981"))
                })
            )
            .onDisappear {
                if !isBeingHidden {
                    hideTerminal()
                }
            }
        }
        .ignoresSafeArea(.keyboard) // Allow keyboard to appear at the navigation view level
    }

    private func hideTerminal() {
        // Set the flag to indicate we're hiding, not closing
        isBeingHidden = true
        // Mark the terminal as minimized in the view model
        terminalViewModel.minimizeTerminal(id: terminal.id)
        // Dismiss the view without closing the terminal
        presentationMode.wrappedValue.dismiss()
    }

    private func closeTerminal() {
        // Clean up the cached terminal view
        TerminalEmulatorView.cleanupTerminalView(id: terminal.id)
        // Close the terminal in the view model
        terminalViewModel.closeTerminal(id: terminal.id)
    }

    private func closeAndDismiss() {
        // First close the terminal
        closeTerminal()
        // Then dismiss the view
        presentationMode.wrappedValue.dismiss()
    }
}

struct TerminalEmulatorView: UIViewRepresentable {
    let terminal: Terminal // Our model's Terminal
    @ObservedObject var terminalViewModel: TerminalViewModel
    let size: CGSize

    // Cache terminal views by terminal ID
    private static var terminalViews: [String: SwiftTerm.TerminalView] = [:]

    func makeUIView(context: Context) -> SwiftTerm.TerminalView {
        // Check if we have a cached terminal view
        if let existingView = Self.terminalViews[terminal.id] {
            return setupExistingTerminalView(existingView, context: context)
        }

        let terminalView = SwiftTerm.TerminalView(frame: .zero)
        terminalView.delegate = context.coordinator

        // Store reference to the terminal view in the coordinator
        context.coordinator.terminalView = terminalView

        // Configure terminal appearance and behavior
        configureTerminalAppearance(terminalView)
        configureTerminalForInput(terminalView, context: context)

        // Cache the terminal view
        Self.terminalViews[terminal.id] = terminalView
        return terminalView
    }

    func updateUIView(_ terminalView: SwiftTerm.TerminalView, context: Context) {
        // Update terminal size based on view size
        let fontSize: CGFloat = 14
        let charWidth: CGFloat = fontSize * 0.6
        let charHeight: CGFloat = fontSize * 1.2

        let cols = Int(size.width / charWidth)
        let rows = Int(size.height / charHeight)

        if cols > 0 && rows > 0 {
            terminalView.resize(cols: cols, rows: rows)
        }
    }

    static func dismantleUIView(_: SwiftTerm.TerminalView, coordinator: Coordinator) {
        // Remove notification observer when view is dismantled
        NotificationCenter.default.removeObserver(coordinator)
    }

    // Add method to clean up terminal views when they're truly done
    static func cleanupTerminalView(id: String) {
        terminalViews.removeValue(forKey: id)
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }

    class Coordinator: NSObject, SwiftTerm.TerminalViewDelegate, UIScrollViewDelegate {
        var parent: TerminalEmulatorView
        weak var terminalView: SwiftTerm.TerminalView?

        init(_ parent: TerminalEmulatorView) {
            self.parent = parent
            super.init()
        }

        @objc func handleTerminalData(_ notification: Notification) {
            guard let userInfo = notification.userInfo,
                  let terminalId = userInfo["terminalId"] as? String,
                  let data = userInfo["data"] as? Data,
                  terminalId == parent.terminal.id
            else {
                return
            }

            DispatchQueue.main.async { [weak self] in
                guard let terminalView = self?.terminalView else { return }

                // Feed data to the terminal
                let byteArray = Array(data)
                let slice = ArraySlice<UInt8>(byteArray)
                terminalView.feed(byteArray: slice)
            }
        }

        // MARK: - Terminal View Delegate Methods

        func send(source _: SwiftTerm.TerminalView, data: ArraySlice<UInt8>) {
            // Send the typed characters to the server
            parent.terminalViewModel.sendData(terminalId: parent.terminal.id, data: Array(data))
        }

        func scrolled(source _: SwiftTerm.TerminalView, position _: Double) {
            // Handle scrolling if needed
        }

        func bell(source _: SwiftTerm.TerminalView) {
            // Handle bell sound if needed
        }

        func clipboardCopy(source _: SwiftTerm.TerminalView, content: Data) {
            #if os(iOS)
                UIPasteboard.general.setData(content, forPasteboardType: "public.utf8-plain-text")
            #elseif os(macOS)
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setData(content, forType: .string)
            #endif
        }

        func hostCurrentDirectoryUpdate(source _: SwiftTerm.TerminalView, directory _: String?) {
            // Handle directory updates if needed
        }

        func setTerminalTitle(source _: SwiftTerm.TerminalView, title _: String) {
            // Update terminal title if needed
        }

        func sizeChanged(source _: SwiftTerm.TerminalView, newCols: Int, newRows: Int) {
            // Send ANSI escape sequence for window size
            if newCols == 0 || newRows == 0 {
                return
            }
            let sizeCommand = "\u{1b}[8;\(newRows);\(newCols)t"
            parent.terminalViewModel.sendData(terminalId: parent.terminal.id, data: Array(sizeCommand.utf8))
        }

        func requestOpenLink(source _: SwiftTerm.TerminalView, link: String, params _: [String: String]) {
            // Handle link opening if needed
            #if os(iOS)
                if let url = URL(string: link) {
                    UIApplication.shared.open(url)
                }
            #endif
        }

        // MARK: - Additional Terminal View Delegate Methods

        func mouseModeChanged(source _: SwiftTerm.TerminalView, mode _: SwiftTerm.Terminal.MouseMode) {
            // Handle mouse mode changes if needed
        }

        func setBackgroundColor(source _: SwiftTerm.TerminalView, color _: SwiftTerm.Color) {
            // Handle background color changes if needed
        }

        func setForegroundColor(source _: SwiftTerm.TerminalView, color _: SwiftTerm.Color) {
            // Handle foreground color changes if needed
        }

        func cursorStyleChanged(source _: SwiftTerm.TerminalView, style _: SwiftTerm.CursorStyle) {
            // Handle cursor style changes if needed
        }

        func colorChanged(source _: SwiftTerm.TerminalView, color _: SwiftTerm.Color) {
            // Handle color changes if needed
        }

        func iTermContent(source _: SwiftTerm.TerminalView, content _: ArraySlice<UInt8>) {
            // Handle iTerm content if needed
        }

        func rangeChanged(source _: SwiftTerm.TerminalView, startY _: Int, endY _: Int) {
            // Handle range changes if needed
        }
    }

    // Helper method to set up an existing terminal view
    private func setupExistingTerminalView(
        _ existingView: SwiftTerm.TerminalView,
        context: Context
    ) -> SwiftTerm.TerminalView {
        context.coordinator.terminalView = existingView

        existingView.isUserInteractionEnabled = true
        _ = existingView.becomeFirstResponder() // Handle unused result

        existingView.terminalDelegate = context.coordinator

        // Set up notification observer for terminal data
        NotificationCenter.default.addObserver(
            context.coordinator,
            selector: #selector(context.coordinator.handleTerminalData(_:)),
            name: .init("TerminalDataReceived"),
            object: nil
        )
        return existingView
    }

    // Helper method to configure terminal appearance
    private func configureTerminalAppearance(_ terminalView: SwiftTerm.TerminalView) {
        // Configure terminal appearance with colors matching the webapp
        terminalView.backgroundColor = UIColor.black

        // Install custom colors to match the webapp
        let colors = [
            // Basic colors (0-7)
            SwiftTerm.Color(hex: "#000000"), // black
            SwiftTerm.Color(hex: "#ef4444"), // red
            SwiftTerm.Color(hex: "#22c55e"), // green
            SwiftTerm.Color(hex: "#f59e0b"), // yellow
            SwiftTerm.Color(hex: "#3b82f6"), // blue
            SwiftTerm.Color(hex: "#8b5cf6"), // magenta
            SwiftTerm.Color(hex: "#06b6d4"), // cyan
            SwiftTerm.Color(hex: "#f3f4f6"), // white

            // Bright colors (8-15)
            SwiftTerm.Color(hex: "#4b5563"), // brightBlack
            SwiftTerm.Color(hex: "#f87171"), // brightRed
            SwiftTerm.Color(hex: "#4ade80"), // brightGreen
            SwiftTerm.Color(hex: "#fbbf24"), // brightYellow
            SwiftTerm.Color(hex: "#60a5fa"), // brightBlue
            SwiftTerm.Color(hex: "#a78bfa"), // brightMagenta
            SwiftTerm.Color(hex: "#22d3ee"), // brightCyan
            SwiftTerm.Color(hex: "#ffffff") // brightWhite
        ]
        terminalView.installColors(colors)

        // Configure foreground, background and cursor colors
        terminalView.nativeBackgroundColor = UIColor.black
        terminalView.nativeForegroundColor = UIColor(hex: "#e5e7eb")
        terminalView.caretColor = UIColor(hex: "#e5e7eb")
    }

    // Helper method to configure terminal for keyboard input
    private func configureTerminalForInput(_ terminalView: SwiftTerm.TerminalView, context: Context) {
        // Configure font
        let fontSize: CGFloat = 14
        if let font = UIFont(name: "Menlo", size: fontSize) {
            terminalView.font = font
        }

        // Configure terminal for keyboard input
        terminalView.isUserInteractionEnabled = true
        _ = terminalView.becomeFirstResponder() // Handle unused result

        // Configure terminal options
        terminalView.terminalDelegate = context.coordinator

        // Set up notification observer for terminal data
        NotificationCenter.default.addObserver(
            context.coordinator,
            selector: #selector(context.coordinator.handleTerminalData(_:)),
            name: .init("TerminalDataReceived"),
            object: nil
        )
    }
}

struct TerminalView_Previews: PreviewProvider {
    static var previews: some View {
        let terminalViewModel = TerminalViewModel()
        let terminal = Terminal(clientId: "client123", title: "Test Terminal")

        return TerminalView(terminal: terminal, terminalViewModel: terminalViewModel)
    }
}
