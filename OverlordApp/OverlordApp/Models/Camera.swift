import Foundation

struct Camera: Identifiable {
    let id: String
    let clientId: String
    var isMinimized: Bool = false
    var position: CGPoint = CGPoint(x: 100, y: 100)
    var size: CGSize = CGSize(width: 400, height: 300)
    
    init(id: String = UUID().uuidString, clientId: String) {
        self.id = id
        self.clientId = clientId
    }
} 