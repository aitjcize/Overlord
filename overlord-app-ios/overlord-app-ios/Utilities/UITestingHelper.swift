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
        // For UI tests that test the login screen, we don't want to bypass auth
        // For UI tests that test post-login functionality, we do want to bypass auth
        ProcessInfo.processInfo.arguments.contains("UI_TESTING_BYPASS_AUTH")
    }

    /// Get a mock token for UI testing
    static var mockToken: String {
        "ui-testing-mock-token"
    }

    /// Setup mock data for UI testing
    static func setupMockData() {
        if isUITesting {
            print("Setting up mock data for UI testing")

            // Set a default server address for all UI tests
            UserDefaults.standard.set("http://localhost:8080", forKey: "serverAddress")

            // Set up any mock data needed for UI tests
            if shouldBypassAuth {
                print("Bypassing authentication for UI testing")
                // Store a mock token to bypass authentication
                UserDefaults.standard.set(mockToken, forKey: "authToken")
            } else {
                print("Not bypassing authentication for UI testing")
                // Clear any existing token to ensure login screen is shown
                UserDefaults.standard.removeObject(forKey: "authToken")
            }
        }
    }
}
