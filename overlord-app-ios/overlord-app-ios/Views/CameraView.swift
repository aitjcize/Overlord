import CoreGraphics
import SwiftUI

struct CameraView: View {
    let camera: Camera
    @ObservedObject var clientViewModel: ClientViewModel
    @Environment(\.presentationMode) var presentationMode

    var body: some View {
        NavigationView {
            ZStack {
                Color(hex: "0f172a").ignoresSafeArea()

                VStack {
                    // Camera feed placeholder
                    ZStack {
                        Color(hex: "1e293b")

                        VStack(spacing: 16) {
                            Image(systemName: "video.fill")
                                .font(.system(size: 60))
                                .foregroundColor(Color(hex: "64748b"))

                            Text("Camera Feed")
                                .font(.title2)
                                .foregroundColor(.white)

                            Text("Client ID: \(camera.clientId)")
                                .font(.caption)
                                .foregroundColor(Color(hex: "94a3b8"))
                        }
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)

                    // Camera controls
                    HStack(spacing: 20) {
                        CameraControlButton(icon: "camera.rotate", color: Color(hex: "3b82f6")) {
                            // Toggle camera
                        }

                        CameraControlButton(icon: "camera.shutter.button", color: Color(hex: "10b981")) {
                            // Take snapshot
                        }

                        CameraControlButton(icon: "record.circle", color: Color(hex: "ef4444")) {
                            // Start/stop recording
                        }
                    }
                    .padding()
                    .background(Color(hex: "1e293b"))
                }
            }
            .navigationBarTitle("Camera Feed", displayMode: .inline)
            .toolbarColorScheme(.dark, for: .navigationBar)
            .toolbarBackground(Color(hex: "1e293b"), for: .navigationBar)
            .toolbarBackground(.visible, for: .navigationBar)
            .navigationBarItems(
                trailing: Button(
                    action: {
                        clientViewModel.removeCamera(id: camera.id)
                        presentationMode.wrappedValue.dismiss()
                    },
                    label: {
                        Text("Close")
                            .foregroundColor(Color(hex: "10b981"))
                    }
                )
            )
        }
    }
}

struct CameraControlButton: View {
    let icon: String
    let color: Color
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: icon)
                .font(.system(size: 24))
                .foregroundColor(.white)
                .frame(width: 60, height: 60)
                .background(color)
                .clipShape(Circle())
        }
    }
}

struct CameraView_Previews: PreviewProvider {
    static var previews: some View {
        CameraView(
            camera: Camera(clientId: "client123"),
            clientViewModel: ClientViewModel()
        )
    }
}
