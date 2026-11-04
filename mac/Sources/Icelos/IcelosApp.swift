import AppKit
import IcelosCore
import SwiftUI

/// IcelosApp is the application entry point.
@main
struct IcelosApp: App {
    /// session owns the backend and the window model.
    @State private var session = Session()
    /// window is the view-only window state the menu commands reach.
    @State private var window = WindowState()

    /// Creates the app and makes it a regular foreground app when run outside a bundle.
    init() {
        NSApplication.shared.setActivationPolicy(.regular)
    }

    var body: some Scene {
        Window("Icelos", id: "main") {
            ContentView(model: session.model, window: window)
                .frame(minWidth: 780, minHeight: 560)
                .themed()
                .onAppear { session.start() }
                .onOpenURL { session.open(urls: [$0]) }
                .onReceive(NotificationCenter.default.publisher(for: NSApplication.willTerminateNotification)) { _ in
                    session.stop()
                }
        }
        .windowToolbarStyle(.unified(showsTitle: false))
        .defaultSize(width: 920, height: 680)
        .commands {
            CommandGroup(replacing: .appInfo) {
                Button("About Icelos") { AboutPanel.show() }
            }
            CommandGroup(replacing: .newItem) {
                Button("Add Files…") { MediaFiles.choose { session.model.add(urls: $0) } }
                    .keyboardShortcut(AppCommand.addFiles.shortcut)
            }
            CommandGroup(replacing: .textEditing) {
                Button("Search") { window.searching = true }
                    .keyboardShortcut(AppCommand.searchFormats.shortcut)
            }
            SidebarCommands()
            CommandGroup(after: .sidebar) {
                Button(window.showsInspector ? "Hide Inspector" : "Show Inspector") {
                    window.showsInspector.toggle()
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

/// AboutPanel shows the standard About panel with the tagline and the bundled notices.
@MainActor
enum AboutPanel {
    /// Shows the panel. The credits link to THIRD_PARTY.md when the bundle carries it, and the
    /// panel's text view opens the link through NSWorkspace in the default viewer.
    static func show() {
        let center = NSMutableParagraphStyle()
        center.alignment = .center
        let base: [NSAttributedString.Key: Any] = [
            .font: NSFont.systemFont(ofSize: NSFont.smallSystemFontSize),
            .foregroundColor: NSColor.labelColor,
            .paragraphStyle: center,
        ]
        let credits = NSMutableAttributedString(string: "Form is never final.", attributes: base)
        if let notices = Bundle.main.url(forResource: "THIRD_PARTY", withExtension: "md") {
            var link = base
            link[.link] = notices
            credits.append(NSAttributedString(string: "\n\n", attributes: base))
            credits.append(NSAttributedString(string: "Third-party software", attributes: link))
        }
        NSApplication.shared.orderFrontStandardAboutPanel(options: [.credits: credits])
        NSApplication.shared.activate()
    }
}
