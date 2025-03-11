import CoreGraphics
import SwiftUI

struct CamerasView: View {
    @ObservedObject var clientViewModel: ClientViewModel
    @State private var showingCamera = false
    @State private var selectedCamera: Camera?

    var body: some View {
        ZStack {
            Color(hex: "0f172a").ignoresSafeArea()

            if clientViewModel.cameras.isEmpty {
                VStack(spacing: 20) {
                    Image(systemName: "video.slash")
                        .font(.system(size: 60))
                        .foregroundColor(Color(hex: "64748b"))

                    Text("No Active Cameras")
                        .font(.title2)
                        .fontWeight(.semibold)
                        .foregroundColor(.white)

                    Text("Open a camera by tapping the camera button on a client")
                        .font(.body)
                        .foregroundColor(Color(hex: "94a3b8"))
                        .multilineTextAlignment(.center)
                        .padding(.horizontal)
                }
            } else {
                List {
                    ForEach(Array(clientViewModel.cameras.values)) { camera in
                        CameraRow(camera: camera, clientViewModel: clientViewModel)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                selectedCamera = camera
                                showingCamera = true
                            }
                    }
                    .onDelete { indexSet in
                        let cameraArray = Array(clientViewModel.cameras.values)
                        for index in indexSet {
                            let camera = cameraArray[index]
                            clientViewModel.removeCamera(id: camera.id)
                        }
                    }
                }
                .listStyle(PlainListStyle())
            }
        }
        .sheet(isPresented: $showingCamera) {
            if let camera = selectedCamera {
                CameraView(camera: camera, clientViewModel: clientViewModel)
            }
        }
    }
}

struct CameraRow: View {
    let camera: Camera
    @ObservedObject var clientViewModel: ClientViewModel

    var clientName: String {
        if let client = clientViewModel.clients.first(where: { $0.mid == camera.clientId }) {
            return client.name ?? client.mid
        }
        return camera.clientId
    }

    var body: some View {
        HStack {
            Image(systemName: "video.fill")
                .font(.system(size: 20))
                .foregroundColor(Color(hex: "3b82f6"))
                .frame(width: 40, height: 40)
                .background(Color(hex: "334155"))
                .cornerRadius(8)

            VStack(alignment: .leading, spacing: 4) {
                Text(clientName)
                    .font(.headline)
                    .foregroundColor(.white)

                Text("Camera Feed")
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
            }

            Spacer()

            Image(systemName: "chevron.right")
                .foregroundColor(Color(hex: "64748b"))
        }
        .padding(.vertical, 8)
        .listRowBackground(Color(hex: "1e293b"))
    }
}

struct CamerasView_Previews: PreviewProvider {
    static var previews: some View {
        let viewModel = ClientViewModel()
        // Add some sample cameras for preview
        _ = viewModel.addCamera(id: "1", clientId: "client1")
        _ = viewModel.addCamera(id: "2", clientId: "client2")

        return CamerasView(clientViewModel: viewModel)
    }
}
