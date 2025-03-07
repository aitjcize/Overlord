import SwiftUI

struct ClientsListView: View {
    @ObservedObject var clientViewModel: ClientViewModel
    @State private var searchText = ""
    @State private var showingClientDetail = false
    @State private var selectedClient: Client?
    
    var body: some View {
        VStack {
            // Search bar
            SearchBar(text: $searchText, placeholder: "Search clients...")
                .padding(.horizontal)
                .onChange(of: searchText) { newValue in
                    clientViewModel.setFilterPattern(newValue)
                }
            
            // Recent clients section
            if !clientViewModel.activeRecentClients.isEmpty {
                VStack(alignment: .leading) {
                    Text("Recent Clients")
                        .font(.headline)
                        .foregroundColor(Color(hex: "94a3b8"))
                        .padding(.horizontal)
                    
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 12) {
                            ForEach(clientViewModel.activeRecentClients) { client in
                                RecentClientCard(client: client) {
                                    selectedClient = client
                                    showingClientDetail = true
                                }
                            }
                        }
                        .padding(.horizontal)
                    }
                }
                .padding(.vertical, 8)
            }
            
            // All clients list
            List {
                ForEach(clientViewModel.filteredClients) { client in
                    ClientRow(client: client)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            selectedClient = client
                            showingClientDetail = true
                        }
                }
            }
            .listStyle(PlainListStyle())
        }
        .background(Color(hex: "0f172a").ignoresSafeArea())
        .sheet(isPresented: $showingClientDetail) {
            if let client = selectedClient {
                ClientDetailView(client: client, clientViewModel: clientViewModel)
            }
        }
    }
}

struct SearchBar: View {
    @Binding var text: String
    var placeholder: String
    
    var body: some View {
        HStack {
            Image(systemName: "magnifyingglass")
                .foregroundColor(Color(hex: "64748b"))
            
            TextField(placeholder, text: $text)
                .foregroundColor(.white)
            
            if !text.isEmpty {
                Button(action: {
                    text = ""
                }) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundColor(Color(hex: "64748b"))
                }
            }
        }
        .padding(10)
        .background(Color(hex: "1e293b"))
        .cornerRadius(10)
    }
}

struct RecentClientCard: View {
    let client: Client
    let action: () -> Void
    
    var body: some View {
        Button(action: action) {
            VStack(alignment: .leading) {
                Text(client.name ?? client.mid)
                    .font(.headline)
                    .foregroundColor(.white)
                    .lineLimit(1)
                
                Text(client.mid)
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
                    .lineLimit(1)
            }
            .frame(width: 150, height: 80)
            .padding()
            .background(Color(hex: "1e293b"))
            .cornerRadius(10)
        }
    }
}

struct ClientRow: View {
    let client: Client
    
    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 4) {
                Text(client.name ?? client.mid)
                    .font(.headline)
                    .foregroundColor(.white)
                
                Text(client.mid)
                    .font(.caption)
                    .foregroundColor(Color(hex: "94a3b8"))
            }
            
            Spacer()
            
            // Camera indicator
            if client.hasCamera {
                Image(systemName: "video.fill")
                    .foregroundColor(Color(hex: "10b981"))
                    .padding(.trailing, 8)
            }
            
            Image(systemName: "chevron.right")
                .foregroundColor(Color(hex: "64748b"))
        }
        .padding(.vertical, 8)
        .listRowBackground(Color(hex: "1e293b"))
    }
}

struct ClientsListView_Previews: PreviewProvider {
    static var previews: some View {
        ClientsListView(clientViewModel: ClientViewModel())
            .preferredColorScheme(.dark)
    }
} 