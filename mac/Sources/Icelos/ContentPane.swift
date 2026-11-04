import AppKit
import IcelosCore
import SwiftUI

/// ContentPane is the middle of the window: the empty state until files arrive, then the file list,
/// with an accent ring while files hover.
struct ContentPane: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// targeted is true while files hover over the window.
    let targeted: Bool
    /// editSettings gives one file its own settings in the inspector.
    let editSettings: (UUID) -> Void
    /// reduceMotion turns off the ring animation.
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        Group {
            if model.items.isEmpty {
                ContentUnavailableView {
                    Label {
                        Text("Drop videos or music here")
                    } icon: {
                        Image(nsImage: NSApplication.shared.applicationIconImage)
                            .resizable()
                            .frame(width: 96, height: 96)
                            .accessibilityHidden(true)
                    }
                } description: {
                    Text("Form is never final.")
                } actions: {
                    Button("Choose Files…") { MediaFiles.choose { model.add(urls: $0) } }
                }
            } else {
                JobList(model: model, editSettings: editSettings)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .modifier(ContentTheme())
        .overlay {
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(Theme.accent.opacity(targeted ? 0.7 : 0), lineWidth: 2)
                .padding(6)
                .allowsHitTesting(false)
                .animation(Motion.animation(.easeOut(duration: 0.2), reduceMotion: reduceMotion), value: targeted)
        }
    }
}

/// ContentTheme paints the pane in the chosen content theme and leaves it to the system under
/// System.
private struct ContentTheme: ViewModifier {
    /// theme is the window's theme.
    @Environment(\.themeName) private var theme

    func body(content: Content) -> some View {
        if theme == .system {
            content
        } else {
            content
                .scrollContentBackground(.hidden)
                .foregroundStyle(Theme.text)
                .background(Theme.background)
        }
    }
}
