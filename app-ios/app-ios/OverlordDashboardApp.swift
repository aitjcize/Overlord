import CoreGraphics
import SwiftTerm
import SwiftUI

@main
struct OverlordDashboardApp: App {
    @StateObject private var authViewModel = AuthViewModel()
    @Environment(\.scenePhase) private var scenePhase

    // Create a static property to track the app's background state
    // This can be checked by other parts of the app
    static var wasInBackground = false

    init() {
        // Setup mock data for UI testing
        UITestingHelper.setupMockData()
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(authViewModel)
        }
        .onChange(of: scenePhase) {
            switch scenePhase {
            case .active:
                // App has become active (either first launch or returning from background)
                if OverlordDashboardApp.wasInBackground {
                    print("App returned to foreground")
                    // Reset the flag
                    OverlordDashboardApp.wasInBackground = false
                }
            case .background:
                // App went to the background
                print("App went to background")
                OverlordDashboardApp.wasInBackground = true
            case .inactive:
                // App is inactive (transitioning between states)
                break
            @unknown default:
                break
            }
        }
    }
}
