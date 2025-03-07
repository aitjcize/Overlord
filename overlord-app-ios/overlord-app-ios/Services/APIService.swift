import Combine
import CoreGraphics
import Foundation

#if os(iOS)
    import UIKit
#elseif os(macOS)
    import AppKit
#endif

@MainActor
class APIService {
    static var baseURL = UserDefaults.standard.string(forKey: "serverAddress")?
        .appending("/api") ?? "http://localhost:8080/api"
    private let session = URLSession.shared

    private var cancellables = Set<AnyCancellable>()

    init() {
        // Listen for server address changes
        NotificationCenter.default.publisher(for: UserDefaults.didChangeNotification)
            .sink { _ in
                if let savedAddress = UserDefaults.standard.string(forKey: "serverAddress") {
                    APIService.baseURL = savedAddress + "/api"
                }
            }
            .store(in: &cancellables)
    }

    func getClients(token: String) -> AnyPublisher<[Client], Error> {
        guard let url = URL(string: "\(APIService.baseURL)/agents/list") else {
            return Fail(error: URLError(.badURL)).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")

        return session.authHandledDataTaskPublisher(for: request)
            .decodeAuth(type: [Client].self)
            .eraseToAnyPublisher()
    }

    func getClientProperties(mid: String, token: String) -> AnyPublisher<[String: String], Error> {
        guard let url = URL(string: "\(APIService.baseURL)/agent/properties/\(mid)") else {
            return Fail(error: URLError(.badURL)).eraseToAnyPublisher()
        }
        var request = URLRequest(url: url)
        request.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")

        return session.authHandledDataTaskPublisher(for: request)
            .decodeAuth(type: [String: String].self)
            .eraseToAnyPublisher()
    }

    func downloadFile(sid: String, token: String) {
        guard let url = URL(string: "\(APIService.baseURL)/file/download/\(sid)?token=\(token)") else { return }

        Task { @MainActor in
            #if os(iOS)
                await UIApplication.shared.open(url)
            #elseif os(macOS)
                NSWorkspace.shared.open(url)
            #endif
        }
    }
}
