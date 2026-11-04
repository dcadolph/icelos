import AppKit
import IcelosCore
import SwiftUI

extension EnvironmentValues {
    /// themeName is the theme the window draws with.
    @Entry var themeName: ThemeName = .standard

    /// palette is the theme's palette for the current appearance and contrast setting.
    var palette: Palette {
        let palette = themeName.palette(dark: colorScheme == .dark)
        return colorSchemeContrast == .increased ? palette.increasedContrast : palette
    }
}

/// ThemeColor is one palette slot that resolves against the window's theme, appearance, and contrast.
struct ThemeColor: ShapeStyle {
    /// slot is the palette color drawn.
    var slot: Palette.Slot
    /// alpha scales the slot's own opacity.
    var alpha: Double = 1

    /// opacity returns the slot drawn at a fraction of its opacity.
    func opacity(_ alpha: Double) -> ThemeColor {
        ThemeColor(slot: slot, alpha: self.alpha * alpha)
    }

    /// resolve returns the slot's color: the system color under the System theme, else the slot in
    /// the environment's palette.
    func resolve(in environment: EnvironmentValues) -> Color {
        if environment.themeName == .system { return Color(nsColor: Self.system(slot)).opacity(alpha) }
        return environment.palette.swatch(slot).color.opacity(alpha)
    }

    /// system returns the AppKit color the System theme draws a slot with, so appearance, accent,
    /// and increased contrast come from the system.
    static func system(_ slot: Palette.Slot) -> NSColor {
        switch slot {
        case .background: .windowBackgroundColor
        case .surface: .controlBackgroundColor
        case .raised: .unemphasizedSelectedContentBackgroundColor
        case .text: .labelColor
        case .muted: .secondaryLabelColor
        case .accent, .accentText: .controlAccentColor
        case .border: .separatorColor
        case .success: .systemGreen
        case .danger: .systemRed
        case .onAccent: .alternateSelectedControlTextColor
        }
    }
}

/// Theme names the palette slots views draw with.
enum Theme {
    /// background is the window base.
    static let background = ThemeColor(slot: .background)
    /// surface is the fill for rows and panels.
    static let surface = ThemeColor(slot: .surface)
    /// raised is the selected row and inspector fill.
    static let raised = ThemeColor(slot: .raised)
    /// text is the primary text color.
    static let text = ThemeColor(slot: .text)
    /// muted is the secondary text color.
    static let muted = ThemeColor(slot: .muted)
    /// accent marks selection, progress, and the primary action.
    static let accent = ThemeColor(slot: .accent)
    /// border is the hairline around surfaces.
    static let border = ThemeColor(slot: .border)
    /// success marks finished work.
    static let success = ThemeColor(slot: .success)
    /// danger marks failures.
    static let danger = ThemeColor(slot: .danger)
    /// onAccent is text drawn on an accent fill.
    static let onAccent = ThemeColor(slot: .onAccent)
    /// accentText is accent-colored text that reads on the background.
    static let accentText = ThemeColor(slot: .accentText)
}

/// ThemeApplier sets the theme environment and the app appearance from the stored settings.
struct ThemeApplier: ViewModifier {
    /// theme is the stored theme choice.
    @AppStorage("theme") private var theme = ThemeName.standard
    /// appearance is the stored light or dark choice.
    @AppStorage("appearance") private var appearance = Appearance.system

    func body(content: Content) -> some View {
        content
            .environment(\.themeName, theme)
            .onChange(of: theme.forcedDark(appearance), initial: true) { _, dark in
                NSApplication.shared.appearance = dark.map { NSAppearance(named: $0 ? .darkAqua : .aqua) } ?? nil
            }
    }
}

extension View {
    /// themed draws the view with the stored theme and appearance.
    func themed() -> some View {
        modifier(ThemeApplier())
    }
}
