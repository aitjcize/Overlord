import Foundation

struct Terminal: Identifiable {
    let id: String
    let clientId: String
    var title: String
    var output: String = ""
    var isMinimized: Bool = false
    var position: CGPoint = CGPoint(x: 100, y: 100)
    var size: CGSize = CGSize(width: 600, height: 400)
    
    init(id: String = UUID().uuidString, clientId: String, title: String) {
        self.id = id
        self.clientId = clientId
        self.title = title
    }
} 