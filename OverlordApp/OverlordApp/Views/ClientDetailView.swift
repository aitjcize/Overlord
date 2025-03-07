import SwiftUI

struct ClientDetailView: View {
    let client: Client
    @ObservedObject var clientViewModel: ClientViewModel
    @State private var showingTerminal = false
    @State private var showingCamera = false
    @Environment(\.presentationMode) var presentationMode
    @StateObject private var terminalViewModel = TerminalViewModel()
    
    var body: some View {
        NavigationView {
            ZStack {
                Color(hex: "0f172a").ignoresSafeArea()
                
                ScrollView {
                    VStack(alignment: .leading, spacing: 20) {
                        // Client header
                        VStack(alignment: .leading, spacing: 8) {
                            Text(client.name ?? "Unnamed Client")
                                .font(.title)
                                .fontWeight(.bold)
                                .foregroundColor(.white)
                            
                            Text(client.mid)
                                .font(.subheadline)
                                .foregroundColor(Color(hex: "94a3b8"))
                        }
                        .padding()
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color(hex: "1e293b"))
                        .cornerRadius(10)
                        
                        // Action buttons
                        HStack(spacing: 15) {
                            ActionButton(
                                title: "Terminal",
                                icon: "terminal",
                                color: Color(hex: "10b981")
                            ) {
                                showingTerminal = true
                            }
                            
                            if client.hasCamera {
                                ActionButton(
                                    title: "Camera",
                                    icon: "video",
                                    color: Color(hex: "3b82f6")
                                ) {
                                    showingCamera = true
                                }
                            }
                            
                            ActionButton(
                                title: "Pin",
                                icon: "pin",
                                color: Color(hex: "f59e0b")
                            ) {
                                clientViewModel.addFixture(client: client)
                                presentationMode.wrappedValue.dismiss()
                            }
                        }
                        .padding(.vertical)
                        
                        // Properties section
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Properties")
                                .font(.headline)
                                .foregroundColor(.white)
                                .padding(.bottom, 4)
                            
                            if let properties = client.properties, !properties.isEmpty {
                                ForEach(properties.sorted(by: { $0.key < $1.key }), id: \.key) { key, value in
                                    PropertyRow(key: key, value: value)
                                }
                            } else {
                                Text("No properties available")
                                    .foregroundColor(Color(hex: "94a3b8"))
                                    .italic()
                            }
                        }
                        .padding()
                        .background(Color(hex: "1e293b"))
                        .cornerRadius(10)
                    }
                    .padding()
                }
            }
            .navigationBarTitle("Client Details", displayMode: .inline)
            .navigationBarItems(trailing: Button(action: {
                presentationMode.wrappedValue.dismiss()
            }) {
                Image(systemName: "xmark.circle.fill")
                    .foregroundColor(Color(hex: "64748b"))
                    .imageScale(.large)
            })
            .sheet(isPresented: $showingTerminal) {
                TerminalView(
                    terminal: terminalViewModel.createTerminal(
                        for: client.mid,
                        title: client.name ?? client.mid
                    ),
                    terminalViewModel: terminalViewModel
                )
            }
            .sheet(isPresented: $showingCamera) {
                if client.hasCamera {
                    let cameraId = UUID().uuidString
                    clientViewModel.addCamera(id: cameraId, clientId: client.mid)
                    if let camera = clientViewModel.cameras[cameraId] {
                        CameraView(camera: camera, clientViewModel: clientViewModel)
                    }
                }
            }
        }
    }
}

struct ActionButton: View {
    let title: String
    let icon: String
    let color: Color
    let action: () -> Void
    
    var body: some View {
        Button(action: action) {
            VStack(spacing: 8) {
                Image(systemName: icon)
                    .font(.system(size: 24))
                    .foregroundColor(color)
                
                Text(title)
                    .font(.caption)
                    .foregroundColor(.white)
            }
            .frame(maxWidth: .infinity)
            .padding()
            .background(Color(hex: "1e293b"))
            .cornerRadius(10)
        }
    }
}

struct PropertyRow: View {
    let key: String
    let value: String
    
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(key)
                .font(.caption)
                .foregroundColor(Color(hex: "94a3b8"))
            
            Text(value)
                .font(.body)
                .foregroundColor(.white)
        }
        .padding(.vertical, 4)
        .padding(.horizontal, 8)
        .background(Color(hex: "334155"))
        .cornerRadius(6)
    }
}

struct ClientDetailView_Previews: PreviewProvider {
    static var previews: some View {
        ClientDetailView(
            client: Client(
                mid: "client123",
                name: "Test Client",
                properties: ["os": "Linux", "version": "1.0.0", "has_camera": "true"]
            ),
            clientViewModel: ClientViewModel()
        )
    }
} 