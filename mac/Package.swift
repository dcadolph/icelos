// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Icelos",
    platforms: [.macOS(.v14)],
    products: [
        .executable(name: "Icelos", targets: ["Icelos"]),
    ],
    targets: [
        .target(name: "IcelosCore"),
        .executableTarget(name: "Icelos", dependencies: ["IcelosCore"]),
        .testTarget(name: "IcelosCoreTests", dependencies: ["IcelosCore"]),
    ]
)
