import SwiftUI
@preconcurrency import WebKit

struct PortForwardsView: View {
    @Environment(\.scenePhase) private var scenePhase
    @ObservedObject var portForwardViewModel: PortForwardViewModel
    @State private var selectedPortForward: PortForward?
    @State private var showingWebView = false
    @State private var isLoading = true
    @State private var currentURL: URL?
    @State private var urlString: String = ""
    @State private var showURLBar: Bool = false
    @State private var canGoBack: Bool = false
    @State private var canGoForward: Bool = false
    @State private var navigationStateTimer: Timer?

    var body: some View {
        ZStack {
            Color(hex: "0f172a").ignoresSafeArea()

            if portForwardViewModel.portForwardsArray.isEmpty {
                VStack(spacing: 20) {
                    Image(systemName: "network")
                        .font(.system(size: 60))
                        .foregroundColor(Color(hex: "64748b"))

                    Text("No Active Port Forwards")
                        .font(.title2)
                        .fontWeight(.semibold)
                        .foregroundColor(.white)

                    Text("Forward a port by tapping the network button on a client")
                        .font(.body)
                        .foregroundColor(Color(hex: "94a3b8"))
                        .multilineTextAlignment(.center)
                        .padding(.horizontal)
                }
            } else {
                List {
                    ForEach(portForwardViewModel.portForwardsArray) { portForward in
                        PortForwardRow(portForward: portForward)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                // Restart the TCP server if needed
                                portForwardViewModel.restartTCPServerIfNeeded(for: portForward.id)

                                // Set the selected port forward first
                                selectedPortForward = portForward

                                // Add a small delay to ensure the port forward is ready
                                DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
                                    showingWebView = true
                                }
                            }
                    }
                    .onDelete { indexSet in
                        for index in indexSet {
                            let portForward = portForwardViewModel.portForwardsArray[index]
                            portForwardViewModel.closePortForward(id: portForward.id)
                        }
                    }
                }
                .listStyle(PlainListStyle())
            }
        }
        .sheet(isPresented: Binding<Bool>(
            get: { showingWebView && selectedPortForward != nil },
            set: { newValue in
                showingWebView = newValue
                if !newValue {
                    selectedPortForward = nil
                }
            }
        ), onDismiss: {
            selectedPortForward = nil
        }, content: {
            if let portForward = selectedPortForward {
                if let url = portForward.localURL {
                    WebViewContainer(
                        url: url,
                        title: portForward.displayName,
                        portForward: portForward,
                        viewModel: portForwardViewModel
                    )
                } else {
                    VStack(spacing: 16) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.system(size: 50))
                            .foregroundColor(.yellow)

                        Text("Invalid URL")
                            .font(.headline)
                            .foregroundColor(.red)

                        // Format the port number without commas
                        let localPortStr = "\(portForward.localPort)"

                        Text("Could not create URL for port forward on port \(localPortStr)")
                            .multilineTextAlignment(.center)
                            .padding(.horizontal)

                        Button("Close") {
                            showingWebView = false
                        }
                        .padding()
                        .background(Color.blue)
                        .foregroundColor(.white)
                        .cornerRadius(8)
                    }
                }
            } else {
                VStack(spacing: 16) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.system(size: 50))
                        .foregroundColor(.yellow)

                    Text("No port forward selected")
                        .font(.headline)
                        .foregroundColor(.red)

                    Button("Close") {
                        showingWebView = false
                    }
                    .padding()
                    .background(Color.blue)
                    .foregroundColor(.white)
                    .cornerRadius(8)
                }
            }
        })
        .onChange(of: scenePhase) {
            switch scenePhase {
            case .active:
                self.portForwardViewModel.restartAllTCPServers()
            default:
                break
            }
        }
    }
}

struct PortForwardRow: View {
    let portForward: PortForward
    @Environment(\.horizontalSizeClass) var horizontalSizeClass

    // Create a formatter that doesn't use thousands separators
    private let portFormatter: NumberFormatter = {
        let formatter = NumberFormatter()
        formatter.usesGroupingSeparator = false
        return formatter
    }()

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "network")
                .font(.system(size: 20))
                .foregroundColor(Color(hex: "3b82f6"))
                .frame(width: 40, height: 40)
                .background(Color(hex: "334155"))
                .cornerRadius(8)

            VStack(alignment: .leading, spacing: 4) {
                Text(portForward.displayName)
                    .font(.headline)
                    .foregroundColor(.white)
                    .lineLimit(1)
                    .truncationMode(.tail)

                HStack {
                    // Format the port numbers without commas
                    let localPortStr = portFormatter
                        .string(from: NSNumber(value: portForward.localPort)) ?? "\(portForward.localPort)"
                    let remotePortStr = portFormatter
                        .string(from: NSNumber(value: portForward.remotePort)) ?? "\(portForward.remotePort)"

                    Text("\(localPortStr) → \(portForward.remoteHost):\(remotePortStr)")
                        .font(.caption)
                        .foregroundColor(Color(hex: "94a3b8"))
                        .lineLimit(1)
                        .truncationMode(.middle)
                }
            }

            Spacer()

            if !portForward.isActive {
                Text("Inactive")
                    .font(.caption)
                    .foregroundColor(Color(hex: "ef4444"))
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Color(hex: "334155"))
                    .cornerRadius(4)
            }
        }
        .padding(.vertical, 8)
        .listRowBackground(Color(hex: "1e293b"))
    }
}

struct WebView: UIViewRepresentable {
    let url: URL
    @Binding var loadError: Error?
    @Binding var isLoading: Bool
    @Binding var currentURL: URL?
    @Binding var canGoBack: Bool
    @Binding var canGoForward: Bool
    @Binding var progressValue: Float

    func makeUIView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.allowsInlineMediaPlayback = true
        configuration.mediaTypesRequiringUserActionForPlayback = []

        // Enable persistent data store to save cookies
        configuration.websiteDataStore = WKWebsiteDataStore.default()
        _ = configuration.websiteDataStore.httpCookieStore

        // Enable cookies
        if #available(iOS 14.0, *) {
            let preferences = WKWebpagePreferences()
            preferences.allowsContentJavaScript = true
            configuration.defaultWebpagePreferences = preferences
        } else {
            // Fallback for iOS 13 and earlier
            configuration.preferences.javaScriptEnabled = true
        }

        // Enable developer extras for debugging
        if #available(iOS 16.4, *) {
            configuration.preferences.isElementFullscreenEnabled = true
        }

        // Create the web view with the configuration
        let webView = WKWebView(frame: .zero, configuration: configuration)
        webView.allowsBackForwardNavigationGestures = true
        webView.allowsLinkPreview = true

        // Set the navigation delegate to handle SSL certificate validation
        webView.navigationDelegate = context.coordinator
        webView.uiDelegate = context.coordinator

        // Add observer for progress
        context.coordinator.addProgressObserver(webView)

        // Load the URL
        if let url = URL(string: url.absoluteString) {
            var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData)

            // Add existing cookies to the request
            if let cookies = HTTPCookieStorage.shared.cookies {
                let cookieDict = HTTPCookie.requestHeaderFields(with: cookies)
                if let cookieHeader = cookieDict["Cookie"] {
                    request.addValue(cookieHeader, forHTTPHeaderField: "Cookie")
                }
            }

            webView.load(request)
        }

        return webView
    }

    func updateUIView(_ webView: WKWebView, context: Context) {
        // Update the current URL if it has changed
        if let currentURL = webView.url, self.currentURL != currentURL {
            self.currentURL = currentURL
        }

        // Update navigation state
        canGoBack = webView.canGoBack
        canGoForward = webView.canGoForward
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }

    class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate {
        var parent: WebView
        private var progressObservation: NSKeyValueObservation?

        init(_ parent: WebView) {
            self.parent = parent
        }

        deinit {
            progressObservation?.invalidate()
        }

        func addProgressObserver(_ webView: WKWebView) {
            progressObservation = webView.observe(\.estimatedProgress, options: [.new]) { [weak self] webView, _ in
                guard let self = self else { return }
                DispatchQueue.main.async {
                    self.parent.progressValue = Float(webView.estimatedProgress)

                    // When progress reaches 1.0, wait a bit and then hide the loading indicator
                    if webView.estimatedProgress >= 1.0 {
                        // Small delay to ensure the page is fully rendered
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
                            if webView.estimatedProgress >= 1.0 {
                                self.parent.isLoading = false
                            }
                        }
                    }
                }
            }
        }

        // Add method to handle cookies
        func configureCookieHandling() {
            // Ensure cookies are synchronized between WKWebView and HTTPCookieStorage
            WKWebsiteDataStore.default().httpCookieStore.getAllCookies { cookies in
                for cookie in cookies {
                    HTTPCookieStorage.shared.setCookie(cookie)
                }
            }
        }

        // Handle SSL certificate validation challenges
        func webView(
            _ webView: WKWebView,
            didReceive challenge: URLAuthenticationChallenge,
            completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void
        ) {
            // Check if this is a server trust challenge (SSL certificate)
            if challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust {
                if let serverTrust = challenge.protectionSpace.serverTrust {
                    // Create a credential from the server trust object
                    let credential = URLCredential(trust: serverTrust)

                    // Accept the certificate and continue
                    completionHandler(.useCredential, credential)
                    return
                }
            }

            // For other types of challenges, perform default handling
            completionHandler(.performDefaultHandling, nil)
        }

        func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) {
            parent.isLoading = true
        }

        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            // Ensure cookies are saved after page load completes
            configureCookieHandling()

            // The loading state will be updated by the progress observer

            // Update current URL and navigation state
            if let url = webView.url {
                parent.currentURL = url
            }
            parent.canGoBack = webView.canGoBack
            parent.canGoForward = webView.canGoForward
        }

        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            parent.loadError = error
            parent.isLoading = false
        }

        func webView(
            _ webView: WKWebView,
            didFailProvisionalNavigation navigation: WKNavigation!,
            withError error: Error
        ) {
            parent.loadError = error
            parent.isLoading = false
        }

        // Handle redirects and decide whether to allow navigation
        func webView(
            _ webView: WKWebView,
            decidePolicyFor navigationAction: WKNavigationAction,
            decisionHandler: @escaping (WKNavigationActionPolicy) -> Void
        ) {
            // Allow all navigation actions
            decisionHandler(.allow)
        }

        // MARK: - WKUIDelegate

        // Handle JavaScript alerts
        func webView(
            _ webView: WKWebView,
            runJavaScriptAlertPanelWithMessage message: String,
            initiatedByFrame frame: WKFrameInfo,
            completionHandler: @escaping () -> Void
        ) {
            // In a real app, you might want to show a native alert here
            completionHandler()
        }

        // Handle JavaScript confirm dialogs
        func webView(
            _ webView: WKWebView,
            runJavaScriptConfirmPanelWithMessage message: String,
            initiatedByFrame frame: WKFrameInfo,
            completionHandler: @escaping (Bool) -> Void
        ) {
            // In a real app, you might want to show a native confirm dialog here
            completionHandler(true)
        }

        // Handle JavaScript text input dialogs
        func webView(
            _ webView: WKWebView,
            runJavaScriptTextInputPanelWithPrompt prompt: String,
            defaultText: String?,
            initiatedByFrame frame: WKFrameInfo,
            completionHandler: @escaping (String?) -> Void
        ) {
            // In a real app, you might want to show a native text input dialog here
            completionHandler(defaultText)
        }

        // Handle new window requests (e.g., target="_blank")
        func webView(
            _ webView: WKWebView,
            createWebViewWith configuration: WKWebViewConfiguration,
            for navigationAction: WKNavigationAction,
            windowFeatures: WKWindowFeatures
        ) -> WKWebView? {
            // Instead of opening a new window, load the URL in the current webview
            if let targetURL = navigationAction.request.url {
                webView.load(URLRequest(url: targetURL))
            }
            return nil
        }
    }
}

struct WebViewContainer: View {
    let url: URL
    let title: String
    let portForward: PortForward
    let viewModel: PortForwardViewModel
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.presentationMode) var presentationMode
    @State private var loadError: Error?
    @State private var isLoading = true
    @State private var currentURL: URL?
    @State private var canGoBack: Bool = false
    @State private var canGoForward: Bool = false
    @State private var navigationStateTimer: Timer?
    @State private var progressValue: Float = 0.0
    @State private var isAnimating: Bool = false

    var body: some View {
        NavigationView {
            VStack(spacing: 0) {
                // Animated loading progress bar
                ZStack(alignment: .leading) {
                    // Background track
                    Rectangle()
                        .frame(height: 3)
                        .foregroundColor(Color.gray.opacity(0.2))

                    // Animated progress bar
                    if isLoading {
                        LoadingProgressBar(progress: $progressValue, isAnimating: $isAnimating)
                            .frame(height: 3)
                    }
                }
                .opacity(isLoading ? 1 : 0)
                .animation(.easeInOut(duration: 0.3), value: isLoading)

                ZStack {
                    if loadError == nil {
                        WebView(
                            url: url,
                            loadError: $loadError,
                            isLoading: $isLoading,
                            currentURL: $currentURL,
                            canGoBack: $canGoBack,
                            canGoForward: $canGoForward,
                            progressValue: $progressValue
                        )
                        .onAppear {
                            currentURL = url
                            isLoading = true
                            progressValue = 0.0
                            startLoadingAnimation()

                            viewModel.restartTCPServerIfNeeded(for: portForward.id)

                            // Start a timer to update navigation state
                            navigationStateTimer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { _ in
                                if let webView = getWebView() {
                                    canGoBack = webView.canGoBack
                                    canGoForward = webView.canGoForward
                                }
                            }
                        }
                        .onDisappear {
                            // Invalidate the timer when the view disappears
                            navigationStateTimer?.invalidate()
                            navigationStateTimer = nil
                        }
                        .onChange(of: currentURL) { _, _ in
                            // We still track URL changes but don't display the URL bar
                        }
                    }

                    if let error = loadError {
                        VStack(spacing: 16) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 50))
                                .foregroundColor(.yellow)

                            Text("Failed to load page")
                                .font(.headline)

                            Text(error.localizedDescription)
                                .font(.body)
                                .multilineTextAlignment(.center)
                                .padding(.horizontal)

                            Text("URL: \(url.absoluteString)")
                                .font(.caption)
                                .foregroundColor(.gray)

                            Button(
                                action: {
                                    print(
                                        "Retrying port forward for client \(portForward.clientId) " +
                                            "to \(portForward.remoteHost):\(portForward.remotePort)"
                                    )
                                    loadError = nil
                                    isLoading = true
                                    progressValue = 0.0
                                    startLoadingAnimation()
                                },
                                label: {
                                    Text("Retry")
                                        .padding(.horizontal, 20)
                                        .padding(.vertical, 10)
                                        .background(Color.blue)
                                        .foregroundColor(.white)
                                        .cornerRadius(8)
                                }
                            )
                        }
                        .padding()
                        .background(Color(.systemBackground))
                        .cornerRadius(12)
                        .shadow(radius: 5)
                    }
                }
            }
            .navigationBarTitle(title, displayMode: .inline)
            .navigationBarItems(
                leading: Button("Close") {
                    // Safely dismiss the view
                    presentationMode.wrappedValue.dismiss()
                },
                trailing: HStack(spacing: 16) {
                    // Back button
                    Button(
                        action: {
                            if let webView = getWebView() {
                                webView.goBack()
                                isLoading = true
                                progressValue = 0.0
                                startLoadingAnimation()
                            }
                        },
                        label: {
                            Image(systemName: "chevron.left")
                        }
                    )
                    .disabled(!canGoBack)
                    .opacity(canGoBack ? 1.0 : 0.5)

                    // Forward button
                    Button(
                        action: {
                            if let webView = getWebView() {
                                webView.goForward()
                                isLoading = true
                                progressValue = 0.0
                                startLoadingAnimation()
                            }
                        },
                        label: {
                            Image(systemName: "chevron.right")
                        }
                    )
                    .disabled(!canGoForward)
                    .opacity(canGoForward ? 1.0 : 0.5)

                    // Menu button with options
                    Menu {
                        // Refresh option
                        Button(
                            action: {
                                if let webView = getWebView() {
                                    webView.reload()
                                    isLoading = true
                                    progressValue = 0.0
                                    startLoadingAnimation()
                                }
                            },
                            label: {
                                Label("Refresh", systemImage: "arrow.clockwise")
                            }
                        )

                        // Clear cookies option
                        Button(
                            action: {
                                clearCookies()
                            },
                            label: {
                                Label("Clear Website Data", systemImage: "trash")
                            }
                        )
                    } label: {
                        Image(systemName: "ellipsis.circle")
                    }
                }
            )
        }
        .onDisappear {
            // Only clear lastCreatedPortForward if we're in the immediate creation flow
            // (shouldShowPortForwardWebView is true), not when browsing existing port forwards
            if viewModel.lastCreatedPortForward?.id == portForward.id && viewModel.shouldShowPortForwardWebView {
                viewModel.lastCreatedPortForward = nil
                viewModel.shouldShowPortForwardWebView = false
            }
        }
        .onChange(of: scenePhase) {
            switch scenePhase {
            case .active:
                self.viewModel.restartTCPServerIfNeeded(for: self.portForward.id)
            default:
                break
            }
        }
    }

    private func startLoadingAnimation() {
        isAnimating = true
    }

    // Function to clear cookies for the current website
    private func clearCookies() {
        guard let webView = getWebView(), let currentURL = currentURL else {
            return
        }

        // Get the host from the current URL
        guard let host = currentURL.host else {
            print("Could not determine host from URL: \(currentURL)")
            return
        }

        showClearDataConfirmationAlert(for: host) { confirmed in
            if confirmed {
                self.performDataClearing(for: host, webView: webView)
            }
        }
    }
}

// MARK: - Loading Progress Bar

struct LoadingProgressBar: View {
    @Binding var progress: Float
    @Binding var isAnimating: Bool
    @State private var animationWidth: CGFloat = 0

    var body: some View {
        GeometryReader { geometry in
            ZStack(alignment: .leading) {
                // Main progress based on actual progress value
                Rectangle()
                    .foregroundColor(Color.blue)
                    .frame(width: geometry.size.width * CGFloat(progress))

                // Animated indeterminate progress indicator
                if progress < 0.9 {
                    Rectangle()
                        .foregroundColor(Color.blue.opacity(0.5))
                        .frame(width: geometry.size.width * 0.2)
                        .offset(x: animationWidth)
                        .onAppear {
                            if isAnimating {
                                withAnimation(Animation.linear(duration: 1.5).repeatForever(autoreverses: false)) {
                                    animationWidth = geometry.size.width * 0.8
                                }
                            }
                        }
                        .onChange(of: isAnimating) { _, newValue in
                            if newValue {
                                withAnimation(Animation.linear(duration: 1.5).repeatForever(autoreverses: false)) {
                                    animationWidth = geometry.size.width * 0.8
                                }
                            } else {
                                animationWidth = 0
                            }
                        }
                }
            }
        }
    }
}

// MARK: - WebViewContainer Helper Methods

extension WebViewContainer {
    // Helper method to get the WKWebView from the view hierarchy
    private func getWebView() -> WKWebView? {
        // Find the WKWebView in the view hierarchy
        // This is a simplified approach - in a real app, you might want to use a more robust method
        if #available(iOS 15.0, *) {
            for scene in UIApplication.shared.connectedScenes {
                if let windowScene = scene as? UIWindowScene {
                    for window in windowScene.windows {
                        if let webView = findWebView(in: window) {
                            return webView
                        }
                    }
                }
            }
        } else {
            // Fallback for iOS 14 and earlier
            for window in UIApplication.shared.windows {
                if let webView = findWebView(in: window) {
                    return webView
                }
            }
        }
        return nil
    }

    private func findWebView(in view: UIView) -> WKWebView? {
        if let webView = view as? WKWebView {
            return webView
        }

        for subview in view.subviews {
            if let webView = findWebView(in: subview) {
                return webView
            }
        }

        return nil
    }

    // Helper method to show confirmation alert
    private func showClearDataConfirmationAlert(for host: String, completion: @escaping (Bool) -> Void) {
        // Create a confirmation alert
        let alert = UIAlertController(
            title: "Clear Website Data",
            message: "Are you sure you want to clear all cookies and website data for \(host)?",
            preferredStyle: .alert
        )

        alert.addAction(UIAlertAction(title: "Cancel", style: .cancel) { _ in
            completion(false)
        })

        alert.addAction(UIAlertAction(title: "Clear", style: .destructive) { _ in
            completion(true)
        })

        // Present the alert
        presentAlert(alert)
    }

    // Helper method to perform the actual data clearing
    private func performDataClearing(for host: String, webView: WKWebView) {
        // Show loading indicator
        let loadingAlert = UIAlertController(
            title: "Clearing Data",
            message: "Please wait...",
            preferredStyle: .alert
        )

        presentAlert(loadingAlert)

        // Clear cookies and website data
        clearCookiesAndData(for: host) {
            // Dismiss loading alert and reload the page
            DispatchQueue.main.async {
                loadingAlert.dismiss(animated: true) {
                    // Reload the page after clearing cookies
                    webView.reload()
                }
            }
        }
    }

    // Helper method to clear cookies and website data
    private func clearCookiesAndData(for host: String, completion: @escaping () -> Void) {
        // Clear cookies for the current website
        WKWebsiteDataStore.default().httpCookieStore.getAllCookies { cookies in
            for cookie in cookies where cookie.domain.contains(host) {
                WKWebsiteDataStore.default().httpCookieStore.delete(cookie) {
                    print("Deleted cookie: \(cookie.name)")
                }
            }

            // Also clear from HTTPCookieStorage
            if let cookies = HTTPCookieStorage.shared.cookies {
                for cookie in cookies where cookie.domain.contains(host) {
                    HTTPCookieStorage.shared.deleteCookie(cookie)
                }
            }

            // Clear all website data types
            let dataTypes = WKWebsiteDataStore.allWebsiteDataTypes()

            WKWebsiteDataStore.default().fetchDataRecords(ofTypes: dataTypes) { records in
                let recordsToRemove = records.filter { record in
                    record.displayName.contains(host)
                }

                WKWebsiteDataStore.default().removeData(
                    ofTypes: dataTypes,
                    for: recordsToRemove
                ) {
                    print("Removed website data for \(host)")
                    completion()
                }
            }
        }
    }

    // Helper method to present an alert
    private func presentAlert(_ alert: UIAlertController) {
        if let windowScene = UIApplication.shared.connectedScenes.first as? UIWindowScene,
           let rootViewController = windowScene.windows.first?.rootViewController
        {
            var currentController = rootViewController
            while let presentedController = currentController.presentedViewController {
                currentController = presentedController
            }
            currentController.present(alert, animated: true)
        }
    }
}
