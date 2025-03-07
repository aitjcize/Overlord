#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import Foundation

struct PortForward: Identifiable, Equatable {
    let id: String
    let clientId: String
    let clientName: String
    let remoteHost: String
    let remotePort: Int
    let localPort: Int
    let useHttps: Bool
    var isActive: Bool = false
    var webSocket: URLSessionWebSocketTask?

    // Implement Equatable
    static func == (lhs: PortForward, rhs: PortForward) -> Bool {
        return lhs.id == rhs.id &&
            lhs.clientId == rhs.clientId &&
            lhs.clientName == rhs.clientName &&
            lhs.remoteHost == rhs.remoteHost &&
            lhs.remotePort == rhs.remotePort &&
            lhs.localPort == rhs.localPort &&
            lhs.useHttps == rhs.useHttps &&
            lhs.isActive == rhs.isActive
        // Note: we don't compare webSocket as it's not relevant for equality
    }

    init(
        id: String = UUID().uuidString,
        clientId: String,
        clientName: String,
        remoteHost: String,
        remotePort: Int,
        localPort: Int,
        useHttps: Bool = false
    ) {
        self.id = id
        self.clientId = clientId
        self.clientName = clientName
        self.remoteHost = remoteHost
        self.remotePort = remotePort
        self.localPort = localPort
        self.useHttps = useHttps
    }

    var displayName: String {
        return "\(clientName) - \(remoteHost):\(remotePort)\(useHttps ? " (https)" : "")"
    }

    var localURL: URL? {
        let urlProtocol = useHttps ? "https" : "http"
        let urlString = "\(urlProtocol)://localhost:\(localPort)"
        let url = URL(string: urlString)
        return url
    }

    // Helper method to get the URL as a string for testing
    var localURLString: String {
        let urlProtocol = useHttps ? "https" : "http"
        return "\(urlProtocol)://localhost:\(localPort)"
    }
}
