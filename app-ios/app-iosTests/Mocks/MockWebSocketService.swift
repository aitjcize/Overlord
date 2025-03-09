@testable import app_ios
import Combine
import Foundation

// Shared MockWebSocketService for testing
class MockWebSocketService: WebSocketService {
    var onEventHandler: (String, @escaping (String) -> Void) -> Void = { _, _ in }
    var startCalled = false
    var stopCalled = false
    var emitCalled = false
    var lastEmittedEvent: String?
    var lastEmittedData: String?

    override func on(event: String, handler: @escaping (String) -> Void) {
        onEventHandler(event, handler)
    }

    override func start(token: String) {
        startCalled = true
    }

    override func stop() {
        stopCalled = true
    }

    func emit(event: String, data: String) {
        emitCalled = true
        lastEmittedEvent = event
        lastEmittedData = data
    }
}
