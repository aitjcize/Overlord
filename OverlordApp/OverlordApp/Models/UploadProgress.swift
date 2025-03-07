import Foundation

struct UploadProgress: Identifiable {
    let id: String
    let filename: String
    let clientId: String
    var progress: Double
    var status: UploadStatus
    var startTime: Date
    
    enum UploadStatus: String {
        case uploading
        case completed
        case failed
    }
    
    init(id: String = UUID().uuidString, filename: String, clientId: String) {
        self.id = id
        self.filename = filename
        self.clientId = clientId
        self.progress = 0.0
        self.status = .uploading
        self.startTime = Date()
    }
} 