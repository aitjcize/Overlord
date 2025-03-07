import Combine
@testable import overlord_app_ios
import XCTest

final class TerminalViewModelTests: XCTestCase {
    var terminalViewModel: TerminalViewModel!
    var mockWebSocketService: MockWebSocketService!
    var cancellables: Set<AnyCancellable>!

    override func setUpWithError() throws {
        mockWebSocketService = MockWebSocketService()
        terminalViewModel = TerminalViewModel(webSocketService: mockWebSocketService)
        cancellables = Set<AnyCancellable>()
    }

    override func tearDownWithError() throws {
        terminalViewModel = nil
        mockWebSocketService = nil
        cancellables = nil
    }

    func testCreateTerminal() {
        // Given
        let clientId = "test-client"
        let title = "Test Terminal"

        // When
        let terminal = terminalViewModel.createTerminal(for: clientId, title: title)

        // Then
        XCTAssertEqual(terminal.clientId, clientId)
        XCTAssertEqual(terminal.title, title)
        XCTAssertEqual(terminal.clientSequentialId, 1)
        XCTAssertEqual(terminalViewModel.terminals.count, 1)
        XCTAssertEqual(terminalViewModel.terminals[terminal.id], terminal)
    }

    func testCreateMultipleTerminalsForSameClient() {
        // Given
        let clientId = "test-client"

        // When
        let terminal1 = terminalViewModel.createTerminal(for: clientId, title: "Terminal 1")
        let terminal2 = terminalViewModel.createTerminal(for: clientId, title: "Terminal 2")

        // Then
        XCTAssertEqual(terminal1.clientSequentialId, 1)
        XCTAssertEqual(terminal2.clientSequentialId, 2)
        XCTAssertTrue(terminalViewModel.hasMultipleTerminals(for: clientId))
    }

    func testCloseTerminal() {
        // Given
        let terminal = terminalViewModel.createTerminal(for: "client", title: "Terminal")
        XCTAssertEqual(terminalViewModel.terminals.count, 1)

        // When
        terminalViewModel.closeTerminal(id: terminal.id)

        // Then
        XCTAssertEqual(terminalViewModel.terminals.count, 0)
        XCTAssertNil(terminalViewModel.terminals[terminal.id])
    }

    func testMinimizeAndMaximizeTerminal() {
        // Given
        let terminal = terminalViewModel.createTerminal(for: "client", title: "Terminal")
        XCTAssertFalse(terminal.isMinimized)

        // When
        let minimizeExpectation = XCTestExpectation(description: "Terminal should be minimized")
        terminalViewModel.minimizeTerminal(id: terminal.id)

        // Then - use RunLoop instead of DispatchQueue for more reliable test execution
        RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.1))
        XCTAssertTrue(terminalViewModel.terminals[terminal.id]?.isMinimized ?? false)
        minimizeExpectation.fulfill()

        // When
        let maximizeExpectation = XCTestExpectation(description: "Terminal should be maximized")
        terminalViewModel.maximizeTerminal(id: terminal.id)

        // Then - use RunLoop instead of DispatchQueue for more reliable test execution
        RunLoop.main.run(until: Date(timeIntervalSinceNow: 0.1))
        XCTAssertFalse(terminalViewModel.terminals[terminal.id]?.isMinimized ?? true)
        maximizeExpectation.fulfill()

        wait(for: [minimizeExpectation, maximizeExpectation], timeout: 1.0)
    }

    func testUpdateTerminalPositionAndSize() {
        // Given
        let terminal = terminalViewModel.createTerminal(for: "client", title: "Terminal")
        let position = CGPoint(x: 100, y: 200)
        let size = CGSize(width: 800, height: 600)

        // When
        terminalViewModel.updateTerminalPosition(id: terminal.id, position: position)
        terminalViewModel.updateTerminalSize(id: terminal.id, size: size)

        // Then
        let updatedTerminal = terminalViewModel.terminals[terminal.id]
        XCTAssertEqual(updatedTerminal?.position.x, position.x)
        XCTAssertEqual(updatedTerminal?.position.y, position.y)
        XCTAssertEqual(updatedTerminal?.size.width, size.width)
        XCTAssertEqual(updatedTerminal?.size.height, size.height)
    }

    func testSetupWebSocketHandlers() {
        // Given
        let expectation = self.expectation(description: "Terminal output handler registered")
        mockWebSocketService.onEventHandler = { event, _ in
            if event == "terminal output" {
                expectation.fulfill()
            }
        }

        // When
        terminalViewModel.setupWebSocketHandlers()

        // Then
        wait(for: [expectation], timeout: 1.0)
    }
}

// Mock WebSocketService for testing
