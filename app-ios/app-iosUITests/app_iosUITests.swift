import XCTest

final class app_iosUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUpWithError() throws {
        // Put setup code here. This method is called before the invocation of each test method in the class.
        continueAfterFailure = false

        app = XCUIApplication()
        app.launchArguments = ["UI_TESTING"]
        app.launch()
    }

    override func tearDownWithError() throws {
        // Put teardown code here. This method is called after the invocation of each test method in the class.
        app = nil
    }

    func testLoginScreen() throws {
        // Verify login screen elements are present
        XCTAssertTrue(app.textFields["Server Address"].exists)
        XCTAssertTrue(app.textFields["Username"].exists)
        XCTAssertTrue(app.secureTextFields["Password"].exists)
        XCTAssertTrue(app.buttons["Login"].exists)

        // Test validation - empty fields
        app.buttons["Login"].tap()

        // Should show error message
        let errorText = app.staticTexts.element(matching: NSPredicate(format: "label CONTAINS %@", "required"))
        XCTAssertTrue(errorText.waitForExistence(timeout: 2))
    }

    func testLoginWithInvalidCredentials() throws {
        // Enter server address
        let serverAddressField = app.textFields["Server Address"]
        serverAddressField.tap()
        serverAddressField.typeText("http://localhost:8080")

        // Enter invalid credentials
        let usernameField = app.textFields["Username"]
        usernameField.tap()
        usernameField.typeText("invalid_user")

        let passwordField = app.secureTextFields["Password"]
        passwordField.tap()
        passwordField.typeText("invalid_password")

        // Tap login button
        app.buttons["Login"].tap()

        // Should show error message
        let errorText = app.staticTexts.element(matching: NSPredicate(format: "label CONTAINS %@", "Invalid"))
        XCTAssertTrue(errorText.waitForExistence(timeout: 5))
    }

    func testNavigationAfterLogin() throws {
        // Note: This test assumes there's a way to bypass authentication for UI testing
        // You would need to implement a mechanism in your app to detect UI_TESTING launch argument
        // and bypass the actual login process

        // For example, you might have code in your app like:
        // if ProcessInfo.processInfo.arguments.contains("UI_TESTING") {
        //     // Bypass authentication and go straight to the dashboard
        // }

        // Assuming we're logged in and on the dashboard
        if app.tabBars.buttons["Clients"].exists {
            // Test tab navigation
            app.tabBars.buttons["Clients"].tap()
            XCTAssertTrue(app.navigationBars["Clients"].exists)

            app.tabBars.buttons["Settings"].tap()
            XCTAssertTrue(app.navigationBars["Settings"].exists)

            // Test logout button exists in settings
            XCTAssertTrue(app.buttons["Logout"].exists)
        } else {
            // Skip this test if we can't bypass authentication
            XCTSkip("This test requires authentication bypass which is not implemented")
        }
    }

    func testLaunchPerformance() throws {
        if #available(macOS 10.15, iOS 13.0, tvOS 13.0, watchOS 7.0, *) {
            // This measures how long it takes to launch your application.
            measure(metrics: [XCTApplicationLaunchMetric()]) {
                XCUIApplication().launch()
            }
        }
    }
}
