import Foundation
import Combine

class UploadProgressViewModel: ObservableObject {
    @Published var records: [UploadProgress] = []
    
    private let webSocketService: WebSocketService
    private var cancellables = Set<AnyCancellable>()
    
    init(webSocketService: WebSocketService = WebSocketService()) {
        self.webSocketService = webSocketService
    }
    
    func setupWebSocketHandlers() {
        webSocketService.on(event: "upload started") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let uploadInfo = try? JSONDecoder().decode(UploadStartInfo.self, from: data) else {
                return
            }
            
            self.addUpload(uploadInfo)
        }
        
        webSocketService.on(event: "upload progress") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let progressInfo = try? JSONDecoder().decode(UploadProgressInfo.self, from: data) else {
                return
            }
            
            self.updateProgress(progressInfo)
        }
        
        webSocketService.on(event: "upload completed") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let completionInfo = try? JSONDecoder().decode(UploadCompletionInfo.self, from: data) else {
                return
            }
            
            self.completeUpload(completionInfo)
        }
        
        webSocketService.on(event: "upload failed") { [weak self] message in
            guard let self = self,
                  let data = message.data(using: .utf8),
                  let failureInfo = try? JSONDecoder().decode(UploadFailureInfo.self, from: data) else {
                return
            }
            
            self.failUpload(failureInfo)
        }
    }
    
    private func addUpload(_ info: UploadStartInfo) {
        DispatchQueue.main.async {
            let upload = UploadProgress(id: info.id, filename: info.filename, clientId: info.clientId)
            self.records.append(upload)
        }
    }
    
    private func updateProgress(_ info: UploadProgressInfo) {
        DispatchQueue.main.async {
            if let index = self.records.firstIndex(where: { $0.id == info.id }) {
                self.records[index].progress = info.progress
            }
        }
    }
    
    private func completeUpload(_ info: UploadCompletionInfo) {
        DispatchQueue.main.async {
            if let index = self.records.firstIndex(where: { $0.id == info.id }) {
                self.records[index].status = .completed
                
                // Remove completed uploads after a delay
                DispatchQueue.main.asyncAfter(deadline: .now() + 3) {
                    self.records.removeAll { $0.id == info.id }
                }
            }
        }
    }
    
    private func failUpload(_ info: UploadFailureInfo) {
        DispatchQueue.main.async {
            if let index = self.records.firstIndex(where: { $0.id == info.id }) {
                self.records[index].status = .failed
                
                // Remove failed uploads after a delay
                DispatchQueue.main.asyncAfter(deadline: .now() + 5) {
                    self.records.removeAll { $0.id == info.id }
                }
            }
        }
    }
}

struct UploadStartInfo: Codable {
    let id: String
    let filename: String
    let clientId: String
}

struct UploadProgressInfo: Codable {
    let id: String
    let progress: Double
}

struct UploadCompletionInfo: Codable {
    let id: String
}

struct UploadFailureInfo: Codable {
    let id: String
    let error: String
} 