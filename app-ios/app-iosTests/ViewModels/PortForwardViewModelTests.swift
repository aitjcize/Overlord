@testable import app_ios
import Combine
import Network
import XCTest

final class PortForwardViewModelTests: XCTestCase {
    var portForwardViewModel: PortForwardViewModel!
    var mockWebSocketService: MockWebSocketService!
    var cancellables: Set<AnyCancellable>!

    override func setUpWithError() throws {
        mockWebSocketService = MockWebSocketService()
        portForwardViewModel = PortForwardViewModel(webSocketService: mockWebSocketService)
        cancellables = Set<AnyCancellable>()
    }

    override func tearDownWithError() throws {
        portForwardViewModel = nil
        mockWebSocketService = nil
        cancellables = nil
    }

    func testCreatePortForward() {
        // Given
        let client = Client(mid: "test-client", name: "Test Client")
        let remoteHost = "example.com"
        let remotePort = 80
        let useHttps = true

        // When
        let portForward = portForwardViewModel.createPortForward(
            for: client,
            remoteHost: remoteHost,
            remotePort: remotePort,
            useHttps: useHttps
        )

        // Then
        XCTAssertEqual(portForward.clientId, client.mid)
        XCTAssertEqual(portForward.clientName, client.name)
        XCTAssertEqual(portForward.remoteHost, remoteHost)
        XCTAssertEqual(portForward.remotePort, remotePort)
        XCTAssertEqual(portForward.useHttps, useHttps)
        XCTAssertEqual(portForwardViewModel.portForwards.count, 1)
        XCTAssertEqual(portForwardViewModel.portForwards[portForward.id], portForward)
        XCTAssertEqual(portForwardViewModel.lastCreatedPortForward, portForward)

        // Wait for the async operation to complete
        let expectation = XCTestExpectation(description: "WebView flag should be set")
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.6) {
            XCTAssertTrue(self.portForwardViewModel.shouldShowPortForwardWebView)
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }

    func testClosePortForward() {
        // Given
        let client = Client(mid: "test-client", name: "Test Client")
        let portForward = portForwardViewModel.createPortForward(
            for: client,
            remoteHost: "example.com",
            remotePort: 80
        )
        XCTAssertEqual(portForwardViewModel.portForwards.count, 1)

        // When
        portForwardViewModel.closePortForward(id: portForward.id)

        // Then
        XCTAssertEqual(portForwardViewModel.portForwards.count, 0)
        XCTAssertNil(portForwardViewModel.portForwards[portForward.id])
        XCTAssertNil(portForwardViewModel.lastCreatedPortForward)
        XCTAssertFalse(portForwardViewModel.shouldShowPortForwardWebView)
    }

    func testCreateMultiplePortForwards() {
        // Given
        let client = Client(mid: "test-client", name: "Test Client")

        // When
        let portForward1 = portForwardViewModel.createPortForward(
            for: client,
            remoteHost: "example1.com",
            remotePort: 80
        )

        let portForward2 = portForwardViewModel.createPortForward(
            for: client,
            remoteHost: "example2.com",
            remotePort: 443,
            useHttps: true
        )

        // Then
        XCTAssertEqual(portForwardViewModel.portForwards.count, 2)
        XCTAssertEqual(portForwardViewModel.portForwardsArray.count, 2)
        XCTAssertEqual(portForwardViewModel.lastCreatedPortForward, portForward2)

        // Verify local ports are different
        XCTAssertNotEqual(portForward1.localPort, portForward2.localPort)
    }

    // Note: Testing the actual TCP and WebSocket connections would require more complex mocking
    // of the Network framework, which is beyond the scope of these basic tests
}

// Define MockWebSocketService if not already defined elsewhere
// Mock WebSocketService for testing
