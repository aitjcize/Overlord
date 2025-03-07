import Combine
import Foundation

extension URLSession {
    /// A custom publisher that handles 401 Unauthorized errors by posting a logout notification
    func authHandledDataTaskPublisher(for request: URLRequest)
    -> AnyPublisher<(data: Data, response: URLResponse), Error> {
        return dataTaskPublisher(for: request)
            .tryMap { data, response in
                // Check if the response is an HTTP response
                guard let httpResponse = response as? HTTPURLResponse else {
                    return (data, response)
                }

                // Check for 401 Unauthorized status code
                if httpResponse.statusCode == 401 {
                    // Post notification to trigger logout
                    DispatchQueue.main.async {
                        NotificationCenter.default.post(name: .logoutRequested, object: nil)
                    }

                    // Throw an error to stop the chain
                    throw URLError(.userAuthenticationRequired)
                }

                return (data, response)
            }
            .eraseToAnyPublisher()
    }
}

// Extension to add a convenience method for decoding JSON responses
extension Publisher where Output == (data: Data, response: URLResponse), Failure == Error {
    func decodeAuth<T: Decodable>(type: T.Type, decoder: JSONDecoder = JSONDecoder()) -> AnyPublisher<T, Error> {
        return map { $0.data }
            .decode(type: type, decoder: decoder)
            .eraseToAnyPublisher()
    }
}
