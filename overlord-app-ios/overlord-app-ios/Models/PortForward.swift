#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import Foundation

struct PortForward: Identifiable {
    let id: String
    let clientId: String
    let clientName: String
    let remoteHost: String
    let remotePort: Int
    let localPort: Int
    let useHttps: Bool
    var isActive: Bool = true
    var webSocket: URLSessionWebSocketTask?

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
        return "\(clientName) - \(remoteHost):\(remotePort)\(useHttps ? " (HTTPS)" : "")"
    }

    var localURL: URL? {
        let urlProtocol = useHttps ? "https" : "http"
        let urlString = "\(urlProtocol)://localhost:\(localPort)"
        let url = URL(string: urlString)
        return url
    }
}
