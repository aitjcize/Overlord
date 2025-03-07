import CoreGraphics
import SwiftTerm
import SwiftUI

@main
struct OverlordAppIOS: App {
    @StateObject private var authViewModel = AuthViewModel()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(authViewModel)
        }
    }
}
