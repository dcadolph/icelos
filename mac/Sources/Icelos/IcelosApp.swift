import AppKit
import IcelosCore
import SwiftUI

/// IcelosApp is the application entry point.
@main
struct IcelosApp: App {
    /// session owns the backend and the window model.
    @State private var session = Session()

    /// Creates the app and makes it a regular foreground app when run outside a bundle.
    init() {
        NSApplication.shared.setActivationPolicy(.regular)
    }

    var body: some Scene {
        Window("Icelos", id: "main") {
            ContentView(model: session.model)
                .frame(minWidth: 780, minHeight: 560)
                .themed()
                .onAppear { session.start() }
                .onReceive(NotificationCenter.default.publisher(for: NSApplication.willTerminateNotification)) { _ in
                    session.stop()
                }
        }
        .windowStyle(.hiddenTitleBar)
        .defaultSize(width: 920, height: 680)
        .commands {
            CommandGroup(replacing: .newItem) {
                Button("Add Files…") { MediaFiles.choose { session.model.add(urls: $0) } }
                    .keyboardShortcut("o")
            }
            CommandMenu("Convert") {
                Button("Convert") { session.model.convertAll() }
                    .keyboardShortcut(.return)
                    .disabled(session.model.pendingCount == 0)
                Button("Clear Finished") { session.model.clearFinished() }
                    .keyboardShortcut("k")
                    .disabled(!session.model.hasFinished)
            }
        }
    }
}

/// Session connects the window model to the converter process.
@MainActor
@Observable
final class Session {
    /// model is the window state.
    let model: Converter
    /// backend is the converter process, or nil when the binary is missing.
    @ObservationIgnored private let backend: Backend?

    /// Creates a session with the located converter.
    init() {
        let backend = Backend.locate().map { Backend(executable: $0) }
        self.backend = backend
        model = Converter(send: { backend?.send($0) })
    }

    /// start launches the converter once.
    func start() {
        guard let backend else {
            model.backendExited("The icelos converter is missing. Reinstall the app, or set ICELOS_BACKEND.")
            return
        }
        guard backend.onMessage == nil else { return }
        let model = model
        backend.onMessage = { model.handle($0) }
        backend.onExit = { model.backendExited($0) }
        do {
            try backend.start()
        } catch {
            model.backendExited("The converter could not start: \(error.localizedDescription)")
        }
    }

    /// stop ends the converter, canceling running jobs.
    func stop() {
        backend?.stop()
    }
}
