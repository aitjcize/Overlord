import Foundation

/// Standard response format from the server
struct StandardResponse<T: Decodable>: Decodable {
    let status: String
    let data: T?
    let message: String?

    // Default implementation of init(from:) for when T conforms to Decodable
    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        status = try container.decode(String.self, forKey: .status)
        message = try container.decodeIfPresent(String.self, forKey: .message)
        data = try container.decodeIfPresent(T.self, forKey: .data)
    }

    enum CodingKeys: String, CodingKey {
        case status, data, message
    }
}
