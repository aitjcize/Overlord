#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import Foundation

struct Terminal: Identifiable, Equatable {
    let id: String
    let clientId: String
    var title: String
    var isMinimized: Bool = false
    var position: CGPoint = .init(x: 100, y: 100)
    var size: CGSize = .init(width: 600, height: 400)
    var cols: Int = 80
    var rows: Int = 24
    var webSocket: URLSessionWebSocketTask?
    var sid: String? // Terminal session ID
    var clientSequentialId: Int = 0 // Sequential ID for terminals with the same client

    // Implement Equatable
    static func == (lhs: Terminal, rhs: Terminal) -> Bool {
        return lhs.id == rhs.id &&
            lhs.clientId == rhs.clientId &&
            lhs.title == rhs.title &&
            lhs.isMinimized == rhs.isMinimized &&
            lhs.cols == rhs.cols &&
            lhs.rows == rhs.rows &&
            lhs.sid == rhs.sid &&
            lhs.clientSequentialId == rhs.clientSequentialId
        // Note: we don't compare webSocket, position, or size as they're not relevant for equality in tests
    }

    init(id: String = UUID().uuidString, clientId: String, title: String, clientSequentialId: Int = 0) {
        self.id = id
        self.clientId = clientId
        self.title = title
        self.clientSequentialId = clientSequentialId
    }
}
