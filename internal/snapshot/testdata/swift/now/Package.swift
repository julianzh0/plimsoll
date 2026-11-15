// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Notes",
    dependencies: [
        .package(url: "https://github.com/apple/swift-argument-parser.git", from: "1.3.0"),
        .package(url: "https://github.com/apple/swift-log", from: "1.5.0"),
    ],
    targets: [
        .executableTarget(
            name: "notes",
            dependencies: ["NotesCore", .product(name: "ArgumentParser", package: "swift-argument-parser")]
        ),
        .target(name: "NotesCore", dependencies: [.product(name: "Logging", package: "swift-log")]),
        .target(name: "Legacy", path: "Old/Legacy"),
        .testTarget(name: "NotesCoreTests", dependencies: ["NotesCore"]),
    ]
)
