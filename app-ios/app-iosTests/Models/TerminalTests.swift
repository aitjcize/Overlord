@testable import app_ios
import XCTest

final class TerminalTests: XCTestCase {
    func testTerminalInitialization() {
        // Given
        let clientId = "test-client-id"
        let title = "Test Terminal"
        let sequentialId = 3

        // When
        let terminal = Terminal(clientId: clientId, title: title, clientSequentialId: sequentialId)

        // Then
        XCTAssertEqual(terminal.clientId, clientId)
        XCTAssertEqual(terminal.title, title)
        XCTAssertEqual(terminal.clientSequentialId, sequentialId)
        XCTAssertFalse(terminal.isMinimized)
        XCTAssertNil(terminal.webSocket)
        XCTAssertNil(terminal.sid)
    }

    func testTerminalMinimization() {
        // Given
        let terminal = Terminal(clientId: "client1", title: "Terminal")

        // When
        var modifiedTerminal = terminal
        modifiedTerminal.isMinimized = true

        // Then
        XCTAssertTrue(modifiedTerminal.isMinimized)
        XCTAssertFalse(terminal.isMinimized)
    }

    func testTerminalPositionAndSize() {
        // Given
        var terminal = Terminal(clientId: "client1", title: "Terminal")
        let position = CGPoint(x: 100, y: 200)
        let size = CGSize(width: 800, height: 600)

        // When
        terminal.position = position
        terminal.size = size

        // Then
        XCTAssertEqual(terminal.position.x, position.x)
        XCTAssertEqual(terminal.position.y, position.y)
        XCTAssertEqual(terminal.size.width, size.width)
        XCTAssertEqual(terminal.size.height, size.height)
    }

    func testTerminalEquality() {
        // Given
        let terminal1 = Terminal(clientId: "client1", title: "Terminal 1")
        let terminal2 = Terminal(clientId: "client1", title: "Terminal 1")

        // Then - terminals should have different IDs even with same parameters
        XCTAssertNotEqual(terminal1, terminal2)
        XCTAssertNotEqual(terminal1.id, terminal2.id)
    }
}
