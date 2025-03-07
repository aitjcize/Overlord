import Foundation

struct Client: Identifiable, Codable {
    var id: String { mid }
    let mid: String
    let name: String?
    var properties: [String: String]?
    var lastSeen: Date
    var hasCamera: Bool
    
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
    }
    
    init(mid: String, name: String? = nil, properties: [String: String]? = nil) {
        self.mid = mid
        self.name = name
        self.properties = properties
        self.lastSeen = Date()
        self.hasCamera = properties?["has_camera"] == "true"
    }
} 