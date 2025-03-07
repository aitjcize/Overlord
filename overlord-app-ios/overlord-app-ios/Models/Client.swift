#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif
import CoreGraphics
import Foundation

struct Client: Identifiable, Codable {
    var id: String { mid }
    let mid: String
    let name: String?
    var properties: [String: String]?
    var lastSeen: Date
    var hasCamera: Bool
    var pinned: Bool

    enum CodingKeys: String, CodingKey {
        case mid, name, properties
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        mid = try container.decode(String.self, forKey: .mid)
        name = try container.decodeIfPresent(String.self, forKey: .name)
        properties = try container.decodeIfPresent([String: String].self, forKey: .properties)
        lastSeen = Date()

        // Determine if client has camera based on properties
        hasCamera = properties?["has_camera"] == "true"
        pinned = false
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(mid, forKey: .mid)
        try container.encodeIfPresent(name, forKey: .name)
        try container.encodeIfPresent(properties, forKey: .properties)
    }

    init(mid: String, name: String? = nil, properties: [String: String]? = nil, pinned: Bool = false) {
        self.mid = mid
        self.name = name
        self.properties = properties
        lastSeen = Date()
        hasCamera = properties?["has_camera"] == "true"
        self.pinned = pinned
    }
}
