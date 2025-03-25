import CoreGraphics
import SwiftTerm
import SwiftUI

@main
struct OverlordDashboardApp: App {
    @StateObject private var authViewModel = AuthViewModel()

    init() {
        // Setup mock data for UI testing
        UITestingHelper.setupMockData()
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(authViewModel)
        }
    }
}
