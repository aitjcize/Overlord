import SwiftUI

struct UploadProgressOverlay: View {
    @ObservedObject var viewModel: UploadProgressViewModel
    
    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack {
                Text("File Uploads")
                    .font(.headline)
                    .foregroundColor(.white)
                
                Spacer()
                
                Text("\(viewModel.records.count) active")
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
            }
            .padding()
            .background(Color(hex: "1e293b"))
            .cornerRadius(10, corners: [.topLeft, .topRight])
            
            // Upload list
            VStack(spacing: 0) {
                ForEach(viewModel.records) { upload in
                    UploadProgressRow(upload: upload)
                        .padding(.horizontal)
                        .padding(.vertical, 8)
                }
            }
            .background(Color(hex: "0f172a"))
            .cornerRadius(10, corners: [.bottomLeft, .bottomRight])
        }
        .frame(maxWidth: 400)
        .shadow(color: Color.black.opacity(0.2), radius: 10, x: 0, y: 5)
    }
}

struct UploadProgressRow: View {
    let upload: UploadProgress
    
    var statusColor: Color {
        switch upload.status {
        case .uploading:
            return Color(hex: "3b82f6")
        case .completed:
            return Color(hex: "10b981")
        case .failed:
            return Color(hex: "ef4444")
        }
    }
    
    var statusIcon: String {
        switch upload.status {
        case .uploading:
            return "arrow.up.circle"
        case .completed:
            return "checkmark.circle"
        case .failed:
            return "xmark.circle"
        }
    }
    
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            // File info
            HStack {
                Image(systemName: statusIcon)
                    .foregroundColor(statusColor)
                
                Text(upload.filename)
                    .font(.subheadline)
                    .foregroundColor(.white)
                    .lineLimit(1)
                
                Spacer()
                
                Text(formattedProgress)
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
            }
            
            // Progress bar
            ProgressView(value: upload.progress, total: 1.0)
                .progressViewStyle(LinearProgressViewStyle(tint: statusColor))
                .frame(height: 4)
        }
    }
    
    var formattedProgress: String {
        switch upload.status {
        case .uploading:
            return "\(Int(upload.progress * 100))%"
        case .completed:
            return "Completed"
        case .failed:
            return "Failed"
        }
    }
}

// Extension to allow rounded corners on specific sides
extension View {
    func cornerRadius(_ radius: CGFloat, corners: UIRectCorner) -> some View {
        clipShape(RoundedCorner(radius: radius, corners: corners))
    }
}

struct RoundedCorner: Shape {
    var radius: CGFloat = .infinity
    var corners: UIRectCorner = .allCorners
    
    func path(in rect: CGRect) -> Path {
        let path = UIBezierPath(
            roundedRect: rect,
            byRoundingCorners: corners,
            cornerRadii: CGSize(width: radius, height: radius)
        )
        return Path(path.cgPath)
    }
}

struct UploadProgressOverlay_Previews: PreviewProvider {
    static var previews: some View {
        ZStack {
            Color(hex: "0f172a").ignoresSafeArea()
            
            UploadProgressOverlay(viewModel: {
                let viewModel = UploadProgressViewModel()
                
                // Add sample uploads
                var upload1 = UploadProgress(id: "1", filename: "document.pdf", clientId: "client1")
                upload1.progress = 0.3
                
                var upload2 = UploadProgress(id: "2", filename: "image.jpg", clientId: "client2")
                upload2.progress = 0.7
                
                var upload3 = UploadProgress(id: "3", filename: "completed.txt", clientId: "client1")
                upload3.progress = 1.0
                upload3.status = .completed
                
                var upload4 = UploadProgress(id: "4", filename: "failed.zip", clientId: "client3")
                upload4.progress = 0.2
                upload4.status = .failed
                
                viewModel.records = [upload1, upload2, upload3, upload4]
                
                return viewModel
            }())
        }
    }
} 