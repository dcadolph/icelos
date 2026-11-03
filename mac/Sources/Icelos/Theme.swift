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

    /// resolve returns the slot's color in the environment's palette.
    func resolve(in environment: EnvironmentValues) -> Color {
        environment.palette.swatch(slot).color.opacity(alpha)
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

    /// backdrop is the window background, under the title bar too.
    static var backdrop: some View {
        Rectangle().fill(background).ignoresSafeArea()
    }

    /// fill is the surface for cards and rows, raised when selected.
    static func fill(selected: Bool = false) -> ThemeColor {
        selected ? raised : surface
    }
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
            .tint(Theme.accent)
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

/// GradientBar is a slim progress bar filled with the accent.
struct GradientBar: View {
    /// value is the fill from 0 to 1.
    var value: Double
    /// reduceMotion turns off the fill animation.
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Capsule().fill(Theme.text.opacity(0.08))
                Capsule().fill(Theme.accent)
                    .frame(width: max(6, geo.size.width * min(1, max(0, value))))
                    .animation(Motion.animation(.easeOut(duration: 0.35), reduceMotion: reduceMotion), value: value)
            }
        }
        .frame(height: 6)
        .accessibilityElement()
        .accessibilityValue(Text("\(Int(value * 100)) percent"))
    }
}
