// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "overlord-app-ios",
    platforms: [
        .iOS(.v17)
    ],
    products: [
        .library(
            name: "overlord-app-ios",
            targets: ["overlord-app-ios"]
        )
    ],
    dependencies: [
        .package(url: "https://github.com/migueldeicaza/SwiftTerm.git", from: "1.2.1")
    ],
    targets: [
        .target(
            name: "overlord-app-ios",
            dependencies: ["SwiftTerm"]
        ),
        .testTarget(
            name: "overlord-app-iosTests",
            dependencies: ["overlord-app-ios"]
        )
    ]
)
