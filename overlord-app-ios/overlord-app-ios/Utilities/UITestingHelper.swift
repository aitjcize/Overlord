import Foundation
import SwiftUI

/// Helper for UI testing to bypass authentication and other processes
enum UITestingHelper {
    /// Check if the app is running in UI testing mode
    static var isUITesting: Bool {
        ProcessInfo.processInfo.arguments.contains("UI_TESTING")
    }

    /// Check if authentication should be bypassed for UI testing
    static var shouldBypassAuth: Bool {
        ProcessInfo.processInfo.arguments.contains("UI_TESTING_BYPASS_AUTH")
    }

    /// Get a mock token for UI testing
    static var mockToken: String {
        "ui-testing-mock-token"
    }

    /// Setup mock data for UI testing
    static func setupMockData() {
        if isUITesting {
            // Set up any mock data needed for UI tests
            if shouldBypassAuth {
                // Store a mock token to bypass authentication
                UserDefaults.standard.set(mockToken, forKey: "authToken")

                // Set a default server address
                UserDefaults.standard.set("http://localhost:8080", forKey: "serverAddress")
            }
        }
    }
}
