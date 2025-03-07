#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import Foundation

struct Camera: Identifiable {
    let id: String
    let clientId: String
    var isMinimized: Bool = false
    var position: CGPoint = .init(x: 100, y: 100)
    var size: CGSize = .init(width: 400, height: 300)

    init(id: String = UUID().uuidString, clientId: String) {
        self.id = id
        self.clientId = clientId
    }
}
