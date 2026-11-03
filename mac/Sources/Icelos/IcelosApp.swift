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
            ContentView(model: session.model, window: session.window)
                .frame(minWidth: 780, minHeight: 560)
                .themed()
                .onAppear { session.start() }
                .onReceive(NotificationCenter.default.publisher(for: NSApplication.willTerminateNotification)) { _ in
                    session.stop()
                }
        }
        .windowToolbarStyle(.unified(showsTitle: false))
        .defaultSize(width: 920, height: 680)
        .commands {
            CommandGroup(replacing: .newItem) {
                Button("Add Files…") { MediaFiles.choose { session.model.add(urls: $0) } }
                    .keyboardShortcut(AppCommand.addFiles.shortcut)
            }
            CommandGroup(replacing: .textEditing) {
                Button("Search Formats") { session.window.searching = true }
                    .keyboardShortcut(AppCommand.searchFormats.shortcut)
            }
            SidebarCommands()
            CommandGroup(after: .sidebar) {
                Button(session.window.showsInspector ? "Hide Inspector" : "Show Inspector") {
                    session.window.showsInspector.toggle()
                }
                .keyboardShortcut(AppCommand.toggleInspector.shortcut)
            }
            CommandMenu("Convert") {
                Button("Convert") { session.model.convertAll() }
                    .keyboardShortcut(AppCommand.convert.shortcut)
                    .disabled(session.model.pendingCount == 0)
                Button("Clear Finished") { session.model.clearFinished() }
                    .keyboardShortcut(AppCommand.clearFinished.shortcut)
                    .disabled(!session.model.hasFinished)
                Divider()
                Button("Remove from List") { session.model.removeSelection() }
                    .keyboardShortcut(AppCommand.removeSelection.shortcut)
                    .disabled(!session.model.canRemoveSelection)
            }
        }
        Settings {
            SettingsView()
        }
    }
}

/// Session connects the window model to the converter process.
@MainActor
@Observable
final class Session {
    /// model is the window state.
    let model: Converter
    /// window is the view-only window state the menu commands reach.
    let window = WindowState()
    /// backend is the converter process, or nil when the binary is missing.
    @ObservationIgnored private let backend: Backend?

    /// Creates a session with the located converter.
    init() {
        let backend = Backend.locate().map { Backend(executable: $0) }
        self.backend = backend
        let settings = AppSettings()
        let model = Converter(send: { backend?.send($0) }, preferences: settings)
        model.outputFolder = settings.outputFolder
        model.hardware = settings.useMediaEngine
        self.model = model
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

/// WindowState holds the view-only state of the window that menu commands also change.
@MainActor
@Observable
final class WindowState {
    /// showsInspector is true while the settings inspector is open.
    var showsInspector = true
    /// search filters the sidebar targets.
    var search = ""
    /// searching is true while the sidebar search field has focus.
    var searching = false
}
