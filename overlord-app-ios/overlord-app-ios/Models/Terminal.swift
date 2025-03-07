#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import Foundation

struct Terminal: Identifiable {
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

    init(id: String = UUID().uuidString, clientId: String, title: String, clientSequentialId: Int = 0) {
        self.id = id
        self.clientId = clientId
        self.title = title
        self.clientSequentialId = clientSequentialId
    }
}
