import XCTest

final class app_iosUITestsLaunchTests: XCTestCase {
    override class var runsForEachTargetApplicationUIConfiguration: Bool {
        true
    }

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testLaunch() throws {
        let app = XCUIApplication()
        app.launch()

        // Take a screenshot of the login screen
        let loginScreenshot = XCTAttachment(screenshot: app.screenshot())
        loginScreenshot.name = "Login Screen"
        loginScreenshot.lifetime = .keepAlways
        add(loginScreenshot)

        // Enter server address (assuming this is required to proceed)
        if app.textFields["Server Address"].exists {
            let serverAddressField = app.textFields["Server Address"]
            serverAddressField.tap()
            serverAddressField.typeText("http://localhost:8080")
        }

        // If we can bypass authentication for UI testing
        if ProcessInfo.processInfo.arguments.contains("UI_TESTING_BYPASS_AUTH") {
            // Assuming we're on the dashboard, take screenshots of key screens

            // Clients tab
            if app.tabBars.buttons["Clients"].exists {
                app.tabBars.buttons["Clients"].tap()
                let clientsScreenshot = XCTAttachment(screenshot: app.screenshot())
                clientsScreenshot.name = "Clients Screen"
                clientsScreenshot.lifetime = .keepAlways
                add(clientsScreenshot)

                // Settings tab
                app.tabBars.buttons["Settings"].tap()
                let settingsScreenshot = XCTAttachment(screenshot: app.screenshot())
                settingsScreenshot.name = "Settings Screen"
                settingsScreenshot.lifetime = .keepAlways
                add(settingsScreenshot)
            }
        }
    }
}
