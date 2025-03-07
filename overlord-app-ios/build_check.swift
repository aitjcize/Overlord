import Foundation

print("Checking for compilation errors...")

@_exported import Combine

// Import all the necessary files to check for compilation errors
@_exported import SwiftUI

// Check Models
class ModelsCheck {
    func check() {
        _ = Client(mid: "test")
        _ = Terminal(clientId: "test", title: "test")
        _ = Camera(clientId: "test")
        _ = UploadProgress(filename: "test", clientId: "test")
    }
}

// Check ViewModels
class ViewModelsCheck {
    func check() {
        _ = AuthViewModel()
        _ = ClientViewModel()
        _ = TerminalViewModel()
        _ = UploadProgressViewModel()
    }
}

// Check Services
class ServicesCheck {
    func check() {
        _ = APIService()
        _ = WebSocketService()
    }
}

// Check Views
class ViewsCheck {
    func check() {
        _ = LoginView()
        _ = DashboardView()
        _ = ClientsListView(clientViewModel: ClientViewModel())
        _ = ClientDetailView(client: Client(mid: "test"), clientViewModel: ClientViewModel())
        _ = TerminalView(terminal: Terminal(clientId: "test", title: "test"), terminalViewModel: TerminalViewModel())
        _ = TerminalsView(terminalViewModel: TerminalViewModel())
        _ = CameraView(camera: Camera(clientId: "test"), clientViewModel: ClientViewModel())
        _ = CamerasView(clientViewModel: ClientViewModel())
        _ = SettingsView()
        _ = UploadProgressOverlay(viewModel: UploadProgressViewModel())
    }
}

print("No compilation errors found!")
