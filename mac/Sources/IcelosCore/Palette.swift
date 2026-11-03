import SwiftUI

/// Swatch is one sRGB color with alpha, in the 0 to 1 range.
public struct Swatch: Hashable, Sendable {
    /// red is the red channel.
    public var red: Double
    /// green is the green channel.
    public var green: Double
    /// blue is the blue channel.
    public var blue: Double
    /// alpha is the opacity.
    public var alpha: Double

    /// black is opaque black.
    public static let black = Swatch(red: 0, green: 0, blue: 0)
    /// white is opaque white.
    public static let white = Swatch(red: 1, green: 1, blue: 1)

    /// Creates a swatch from channel values in the 0 to 1 range.
    public init(red: Double, green: Double, blue: Double, alpha: Double = 1) {
        self.red = red
        self.green = green
        self.blue = blue
        self.alpha = alpha
    }

    /// Creates a swatch from a "#RRGGBB" or "RRGGBB" string, or returns nil when it is not one.
    public init?(hex: String) {
        let digits = hex.hasPrefix("#") ? hex.dropFirst() : Substring(hex)
        guard digits.count == 6, let value = UInt32(digits, radix: 16) else { return nil }
        self.init(red: Double(value >> 16 & 0xFF) / 255, green: Double(value >> 8 & 0xFF) / 255,
                  blue: Double(value & 0xFF) / 255)
    }

    /// luminance is the WCAG relative luminance of the opaque color.
    public var luminance: Double {
        func linear(_ c: Double) -> Double {
            c <= 0.04045 ? c / 12.92 : pow((c + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * linear(red) + 0.7152 * linear(green) + 0.0722 * linear(blue)
    }

    /// contrast returns the WCAG contrast ratio between the two opaque colors.
    public func contrast(with other: Swatch) -> Double {
        let (lo, hi) = luminance < other.luminance ? (luminance, other.luminance) : (other.luminance, luminance)
        return (hi + 0.05) / (lo + 0.05)
    }

    /// opacity returns the same color at the given alpha.
    public func opacity(_ alpha: Double) -> Swatch {
        Swatch(red: red, green: green, blue: blue, alpha: alpha)
    }

    /// mixed returns the opaque color a fraction of the way toward other.
    public func mixed(with other: Swatch, by fraction: Double) -> Swatch {
        Swatch(red: red + (other.red - red) * fraction, green: green + (other.green - green) * fraction,
               blue: blue + (other.blue - blue) * fraction)
    }

    /// color is the SwiftUI color for the swatch.
    public var color: Color {
        Color(red: red, green: green, blue: blue, opacity: alpha)
    }
}

/// Appearance is the light or dark choice the user makes in Settings.
public enum Appearance: String, Codable, CaseIterable, Sendable {
    /// system follows the system appearance.
    case system
    /// light always uses the light palette.
    case light
    /// dark always uses the dark palette.
    case dark

    /// name is the label shown in Settings.
    public var name: String {
        switch self {
        case .system: "System"
        case .light: "Light"
        case .dark: "Dark"
        }
    }
}

/// Palette holds the color slots one theme fills for one appearance.
public struct Palette: Hashable, Sendable {
    /// Slot names one color of a palette.
    public enum Slot: String, CaseIterable, Sendable {
        /// background is the window base.
        case background
        /// surface is the fill for rows and panels.
        case surface
        /// raised is the selected row and inspector fill.
        case raised
        /// text is the primary text color.
        case text
        /// muted is the secondary text color.
        case muted
        /// accent is the brand color for selection, progress, and the primary action.
        case accent
        /// border is the hairline around surfaces.
        case border
        /// success marks finished work.
        case success
        /// danger marks failures.
        case danger
        /// onAccent is text drawn on an accent fill.
        case onAccent
        /// accentText is accent-colored text on the background.
        case accentText
    }

    /// background is the window base.
    public var background: Swatch
    /// surface is the fill for rows and panels.
    public var surface: Swatch
    /// raised is the selected row and inspector fill.
    public var raised: Swatch
    /// text is the primary text color.
    public var text: Swatch
    /// muted is the secondary text color.
    public var muted: Swatch
    /// accent is the brand color for selection, progress, and the primary action.
    public var accent: Swatch
    /// border is the hairline around surfaces, text at a low alpha.
    public var border: Swatch
    /// success marks finished work.
    public var success: Swatch
    /// danger marks failures.
    public var danger: Swatch
    /// isDark is true when the palette is drawn light on dark.
    public var isDark: Bool

    /// Creates a palette from its slots. Raised defaults to surface moved 6% toward text, and border
    /// to text at 10% on dark and 12% on light.
    public init(background: Swatch, surface: Swatch, raised: Swatch? = nil, text: Swatch, muted: Swatch,
                accent: Swatch, success: Swatch, danger: Swatch, isDark: Bool) {
        self.background = background
        self.surface = surface
        self.raised = raised ?? surface.mixed(with: text, by: 0.06)
        self.text = text
        self.muted = muted
        self.accent = accent
        border = text.opacity(isDark ? 0.10 : 0.12)
        self.success = success
        self.danger = danger
        self.isDark = isDark
    }

    /// onAccent is the background when it reads on the accent at 4.5, otherwise black or white,
    /// whichever reads better.
    public var onAccent: Swatch {
        if background.contrast(with: accent) >= 4.5 { return background }
        return Swatch.black.contrast(with: accent) >= Swatch.white.contrast(with: accent) ? .black : .white
    }

    /// accentText is the accent when it reads on the background at 4.5, otherwise the text color.
    public var accentText: Swatch {
        accent.contrast(with: background) >= 4.5 ? accent : text
    }

    /// increasedContrast returns the palette for the system's increased contrast setting: muted text
    /// becomes the text color and borders rise to 35%.
    public var increasedContrast: Palette {
        var copy = self
        copy.muted = text
        copy.border = text.opacity(0.35)
        return copy
    }

    /// swatch returns the color in one slot.
    public func swatch(_ slot: Slot) -> Swatch {
        switch slot {
        case .background: background
        case .surface: surface
        case .raised: raised
        case .text: text
        case .muted: muted
        case .accent: accent
        case .border: border
        case .success: success
        case .danger: danger
        case .onAccent: onAccent
        case .accentText: accentText
        }
    }
}

/// ThemeName is one of the themes the user can pick in Settings.
public enum ThemeName: String, Codable, CaseIterable, Sendable {
    /// obsidianGold is the default brand theme.
    case obsidianGold
    /// harbor is the neutral theme with a teal accent.
    case harbor
    /// dragonsgale is orange on black.
    case dragonsgale
    /// greekFire is amber flame on soot.
    case greekFire
    /// phosphor is a green phosphor terminal.
    case phosphor
    /// redcoat is scarlet on black.
    case redcoat
    /// hussar is crimson and gold.
    case hussar
    /// tyrian is Phoenician purple.
    case tyrian
    /// ultramarine is Renaissance blue.
    case ultramarine
    /// verdigris is copper patina teal.
    case verdigris
    /// oldIronsides is copper on iron.
    case oldIronsides
    /// bletchley is teleprinter paper and ink.
    case bletchley

    /// standard is the theme used when none is chosen or the stored name is unknown.
    public static let standard = ThemeName.obsidianGold

    /// Creates the theme for a stored name, falling back to the standard theme.
    public init(stored: String?) {
        self = stored.flatMap(ThemeName.init(rawValue:)) ?? .standard
    }

    /// name is the label shown in Settings.
    public var name: String {
        switch self {
        case .obsidianGold: "Obsidian & Gold"
        case .harbor: "Harbor"
        case .dragonsgale: "Dragonsgale"
        case .greekFire: "Greek Fire"
        case .phosphor: "Phosphor"
        case .redcoat: "Redcoat"
        case .hussar: "Hussar"
        case .tyrian: "Tyrian"
        case .ultramarine: "Ultramarine"
        case .verdigris: "Verdigris"
        case .oldIronsides: "Old Ironsides"
        case .bletchley: "Bletchley"
        }
    }

    /// followsAppearance is true for themes with both a light and a dark palette.
    public var followsAppearance: Bool {
        self == .obsidianGold || self == .harbor
    }

    /// forcedDark returns whether the theme draws dark for the appearance, or nil when it follows
    /// the system.
    public func forcedDark(_ appearance: Appearance) -> Bool? {
        guard followsAppearance else { return palette(dark: true).isDark }
        switch appearance {
        case .system: return nil
        case .light: return false
        case .dark: return true
        }
    }

    /// palette returns the theme's palette. Themes without a light and dark pair ignore dark.
    public func palette(dark: Bool) -> Palette {
        switch self {
        case .obsidianGold:
            dark
                ? Palette(hex: ("0B0B0B", "151517", "1E1E21", "F7F4E9", "A39E92", "C9A24B"), dark: true)
                : Palette(hex: ("F6F3EC", "FFFFFF", "EFEBE1", "1A1916", "67635A", "8A6A1E"), dark: false)
        case .harbor:
            dark
                ? Palette(hex: ("1c1c1e", "2a2a2d", nil, "f2f2f2", "a1a1a6", "4fb3bb"), dark: true)
                : Palette(hex: ("f6f6f4", "ffffff", nil, "1d1d1f", "6e6e73", "2b8a92"), dark: false)
        case .dragonsgale: Palette(hex: ("0d0a08", "1a1310", nil, "f4e6d8", "b89c86", "ff7a1a"), dark: true)
        case .greekFire: Palette(hex: ("120805", "24100a", nil, "ffe9c2", "c99a6b", "ffb000"), dark: true)
        case .phosphor: Palette(hex: ("050a05", "0b160b", nil, "b8ffb0", "5fae5a", "33ff33"), dark: true)
        case .redcoat: Palette(hex: ("0e0c0c", "1c1919", nil, "f4f1ee", "a9a3a0", "d6282f"), dark: true)
        case .hussar: Palette(hex: ("1a0608", "2b0c10", nil, "f3e3c0", "c2a272", "d4a63a"), dark: true)
        case .tyrian: Palette(hex: ("140816", "231027", nil, "f0e2f5", "b597bf", "b04cc8"), dark: true)
        case .ultramarine: Palette(hex: ("060b1f", "0e1736", nil, "e3e9ff", "8e9cc9", "4f78ff"), dark: true)
        case .verdigris: Palette(hex: ("0a1614", "12241f", nil, "dff3ec", "86b3a5", "43b39a"), dark: true)
        case .oldIronsides: Palette(hex: ("0c0c0d", "18181a", nil, "f5f5f2", "a8a8a3", "c07a44"), dark: true)
        case .bletchley:
            Palette(hex: ("ece4cf", "f6f0dd", nil, "1a1a17", "5c5748", "7a2e12"), dark: false,
                    success: "1A6E45")
        }
    }
}

extension Palette {
    /// darkSuccess and the other semantic defaults fill the slots the owner's themes do not list.
    private static let darkSuccess = "3FB37A", darkDanger = "E0604F", lightSuccess = "1F7A4D",
                       lightDanger = "B3261E"

    /// Creates a palette from hex strings for background, surface, raised (nil derives it), text,
    /// muted, and accent, with the semantic colors for the appearance unless overridden.
    init(hex: (String, String, String?, String, String, String), dark: Bool, success: String? = nil) {
        func s(_ hex: String) -> Swatch {
            guard let swatch = Swatch(hex: hex) else { preconditionFailure("Palette: bad hex \(hex)") }
            return swatch
        }
        self.init(background: s(hex.0), surface: s(hex.1), raised: hex.2.map(s), text: s(hex.3), muted: s(hex.4),
                  accent: s(hex.5),
                  success: s(success ?? (dark ? Self.darkSuccess : Self.lightSuccess)),
                  danger: s(dark ? Self.darkDanger : Self.lightDanger), isDark: dark)
    }
}
