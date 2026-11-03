import SwiftUI

/// AppCommand is one menu command with its keyboard shortcut, so the menus and the tests read the
/// same table.
public enum AppCommand: String, CaseIterable, Sendable {
    case addFiles
    case convert
    case clearFinished
    case toggleInspector
    case searchFormats
    case removeSelection

    /// key is the shortcut's key.
    public var key: KeyEquivalent {
        switch self {
        case .addFiles: "o"
        case .convert: .return
        case .clearFinished: "k"
        case .toggleInspector: "i"
        case .searchFormats: "f"
        case .removeSelection: .delete
        }
    }

    /// modifiers are the shortcut's modifier keys.
    public var modifiers: EventModifiers {
        self == .toggleInspector ? [.command, .option] : .command
    }

    /// shortcut is the key and modifiers for keyboardShortcut.
    public var shortcut: KeyboardShortcut {
        KeyboardShortcut(key, modifiers: modifiers)
    }

    /// display is the shortcut as a menu shows it, e.g. "⌥⌘I".
    public var display: String {
        let mods = (modifiers.contains(.option) ? "⌥" : "") + (modifiers.contains(.command) ? "⌘" : "")
        let name = switch key {
        case .return: "↩"
        case .delete: "⌫"
        default: String(key.character).uppercased()
        }
        return mods + name
    }
}
