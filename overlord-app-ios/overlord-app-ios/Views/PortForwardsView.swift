import SwiftUI
@preconcurrency import WebKit

struct PortForwardsView: View {
    @ObservedObject var portForwardViewModel: PortForwardViewModel
    @State private var selectedPortForward: PortForward?
    @State private var showingWebView = false

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
    }
}

struct PortForwardRow: View {
    let portForward: PortForward

    // Create a formatter that doesn't use thousands separators
    private let portFormatter: NumberFormatter = {
        let formatter = NumberFormatter()
        formatter.usesGroupingSeparator = false
        return formatter
    }()

    var body: some View {
        HStack {
            Image(systemName: "network")
                .font(.system(size: 20))
                .foregroundColor(portForward.isActive ? Color(hex: "3b82f6") : Color(hex: "ef4444"))
                .frame(width: 40, height: 40)
                .background(Color(hex: "334155"))
                .cornerRadius(8)

            VStack(alignment: .leading, spacing: 4) {
                Text(portForward.displayName)
                    .font(.headline)
                    .foregroundColor(.white)

                HStack {
                    // Format the port numbers without commas
                    let localPortStr = portFormatter
                        .string(from: NSNumber(value: portForward.localPort)) ?? "\(portForward.localPort)"
                    let remotePortStr = portFormatter
                        .string(from: NSNumber(value: portForward.remotePort)) ?? "\(portForward.remotePort)"

                    Text("localhost:\(localPortStr) → \(portForward.remoteHost):\(remotePortStr)")
                        .font(.caption)
                        .foregroundColor(Color(hex: "94a3b8"))

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
            }
        }
        .padding(.vertical, 8)
        .listRowBackground(Color(hex: "1e293b"))
    }
}

struct WebView: UIViewRepresentable {
    let url: URL
    var onError: ((Error) -> Void)?

    func makeUIView(context: Context) -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.allowsInlineMediaPlayback = true
        configuration.mediaTypesRequiringUserActionForPlayback = []

        // Enable persistent data store to save cookies
        configuration.websiteDataStore = WKWebsiteDataStore.default()
        _ = configuration.websiteDataStore.httpCookieStore

        // Enable cookies
        configuration.preferences.javaScriptEnabled = true

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

    func updateUIView(_ webView: WKWebView, context: Context) {}

    func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }

    class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate {
        var parent: WebView

        init(_ parent: WebView) {
            self.parent = parent
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

        func webView(_ webView: WKWebView, didStartProvisionalNavigation navigation: WKNavigation!) {}

        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            // Ensure cookies are saved after page load completes
            configureCookieHandling()
        }

        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            parent.onError?(error)
        }

        func webView(
            _ webView: WKWebView,
            didFailProvisionalNavigation navigation: WKNavigation!,
            withError error: Error
        ) {
            parent.onError?(error)
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
    @Environment(\.presentationMode) var presentationMode
    @State private var loadError: Error?
    @State private var isLoading = true
    @State private var currentURL: URL?
    @State private var urlString: String = ""
    @State private var showURLBar: Bool = false

    var body: some View {
        NavigationView {
            VStack(spacing: 0) {
                // URL Bar (shown when tapping the title)
                if showURLBar {
                    HStack {
                        TextField("URL", text: $urlString, onCommit: {
                            loadURL()
                        })
                        .keyboardType(.URL)
                        .autocapitalization(.none)
                        .disableAutocorrection(true)
                        .padding(8)
                        .background(Color(.systemGray6))
                        .cornerRadius(8)

                        Button(
                            action: {
                                loadURL()
                            },
                            label: {
                                Image(systemName: "arrow.right.circle.fill")
                                    .foregroundColor(.blue)
                            }
                        )
                        .padding(.horizontal, 8)
                    }
                    .padding(.horizontal)
                    .padding(.vertical, 8)
                    .background(Color(.systemBackground))
                }

                ZStack {
                    if loadError == nil {
                        WebView(url: url, onError: { error in
                            print("WebView reported error: \(error.localizedDescription)")
                            loadError = error
                        })
                        .onAppear {
                            currentURL = url
                            urlString = url.absoluteString
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
            .navigationBarTitle(showURLBar ? "" : (currentURL?.host ?? title), displayMode: .inline)
            .navigationBarItems(
                leading: HStack {
                    Button("Close") {
                        // Safely dismiss the view
                        presentationMode.wrappedValue.dismiss()
                    }

                    // Toggle URL bar button
                    Button(
                        action: {
                            showURLBar.toggle()
                        },
                        label: {
                            Image(systemName: showURLBar ? "chevron.up" : "chevron.down")
                        }
                    )
                },
                trailing: HStack(spacing: 16) {
                    // Back button
                    Button(
                        action: {
                            if let webView = getWebView() {
                                webView.goBack()
                            }
                        },
                        label: {
                            Image(systemName: "chevron.left")
                        }
                    )

                    // Forward button
                    Button(
                        action: {
                            if let webView = getWebView() {
                                webView.goForward()
                            }
                        },
                        label: {
                            Image(systemName: "chevron.right")
                        }
                    )

                    // Refresh button
                    Button(
                        action: {
                            if let webView = getWebView() {
                                webView.reload()
                            }
                        },
                        label: {
                            Image(systemName: "arrow.clockwise")
                        }
                    )

                    // Open in Safari button
                    Button(
                        action: {
                            if let currentURL = currentURL {
                                UIApplication.shared.open(currentURL)
                            } else {
                                UIApplication.shared.open(url)
                            }
                        },
                        label: {
                            Image(systemName: "safari")
                        }
                    )
                }
            )
        }
        .onDisappear {
            // Ensure we clean up when the view disappears
            if viewModel.lastCreatedPortForward?.id == portForward.id {
                viewModel.lastCreatedPortForward = nil
                viewModel.shouldShowPortForwardWebView = false
            }
        }
    }

    private func loadURL() {
        guard let url = URL(string: urlString) else {
            print("Invalid URL: \(urlString)")
            return
        }

        if let webView = getWebView() {
            webView.load(URLRequest(url: url))
        }

        // Hide keyboard
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }

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
}
