@testable import app_ios
import Combine
import XCTest

// Create a testable subclass of AuthViewModel that doesn't check UITestingHelper
class TestableAuthViewModel: AuthViewModel {
    override init() {
        super.init()
        // Reset state to ensure tests start with a clean state
        isAuthenticated = false
        token = nil
        isLoading = false
        error = nil
    }
}

final class AuthViewModelTests: XCTestCase {
    var authViewModel: TestableAuthViewModel!
    var cancellables: Set<AnyCancellable>!

    override func setUpWithError() throws {
        authViewModel = TestableAuthViewModel()
        cancellables = Set<AnyCancellable>()

        // Clear any saved token
        UserDefaults.standard.removeObject(forKey: "authToken")
    }

    override func tearDownWithError() throws {
        authViewModel = nil
        cancellables = nil
        UserDefaults.standard.removeObject(forKey: "authToken")
    }

    func testInitialState() {
        // Then
        XCTAssertFalse(authViewModel.isAuthenticated)
        XCTAssertNil(authViewModel.token)
        XCTAssertFalse(authViewModel.isLoading)
        XCTAssertNil(authViewModel.error)
    }

    func testInitWithSavedToken() {
        // Given
        let savedToken = "test-token-123"
        UserDefaults.standard.set(savedToken, forKey: "authToken")

        // When
        let viewModel = AuthViewModel()

        // Then
        XCTAssertTrue(viewModel.isAuthenticated)
        XCTAssertEqual(viewModel.token, savedToken)
    }

    func testLogout() {
        // Given
        let savedToken = "test-token-123"
        UserDefaults.standard.set(savedToken, forKey: "authToken")
        let viewModel = AuthViewModel()
        XCTAssertTrue(viewModel.isAuthenticated)

        // When
        viewModel.logout()

        // Then
        XCTAssertFalse(viewModel.isAuthenticated)
        XCTAssertNil(viewModel.token)
        XCTAssertNil(UserDefaults.standard.string(forKey: "authToken"))
    }

    func testLoginValidation() {
        // Given
        // No setup needed

        // When - empty username
        authViewModel.login(username: "", password: "password")

        // Then
        XCTAssertNotNil(authViewModel.error)
        XCTAssertFalse(authViewModel.isLoading)

        // Reset
        authViewModel.error = nil

        // When - empty password
        authViewModel.login(username: "username", password: "")

        // Then
        XCTAssertNotNil(authViewModel.error)
        XCTAssertFalse(authViewModel.isLoading)
    }

    // Note: Testing actual login would require mocking the network layer
    // This would be a more advanced test that would mock URLSession
}
