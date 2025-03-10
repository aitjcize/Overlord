import Foundation
import Combine

class APIService {
    static let baseURL = "http://your-server-address/api" // Replace with your actual server address
    
    private let session: URLSession
    private var cancellables = Set<AnyCancellable>()
    
    init(session: URLSession = .shared) {
        self.session = session
    }
    
    func getClients(token: String) -> AnyPublisher<[Client], Error> {
        let url = URL(string: "\(APIService.baseURL)/agents")!
        var request = URLRequest(url: url)
        request.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        
        return session.dataTaskPublisher(for: request)
            .map { $0.data }
            .decode(type: [Client].self, decoder: JSONDecoder())
            .eraseToAnyPublisher()
    }
    
    func getClientProperties(mid: String, token: String) -> AnyPublisher<[String: String], Error> {
        let url = URL(string: "\(APIService.baseURL)/agents/\(mid)/properties")!
        var request = URLRequest(url: url)
        request.addValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        
        return session.dataTaskPublisher(for: request)
            .map { $0.data }
            .decode(type: [String: String].self, decoder: JSONDecoder())
            .eraseToAnyPublisher()
    }
    
    func downloadFile(sid: String, token: String) {
        let url = URL(string: "\(APIService.baseURL)/sessions/\(sid)/file?token=\(token)")!
        UIApplication.shared.open(url)
    }
} 