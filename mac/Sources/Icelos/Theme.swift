import SwiftUI

/// Theme holds the icelos palette: bone white to ice blue to emerald on midnight.
enum Theme {
    /// midnight is the window base.
    static let midnight = Color(red: 0x07 / 255, green: 0x16 / 255, blue: 0x1B / 255)
    /// deep is the raised surface color.
    static let deep = Color(red: 0x0D / 255, green: 0x27 / 255, blue: 0x30 / 255)
    /// bone is the primary text color.
    static let bone = Color(red: 0xF2 / 255, green: 0xEE / 255, blue: 0xE6 / 255)
    /// ice is the accent color.
    static let ice = Color(red: 0x9F / 255, green: 0xD8 / 255, blue: 0xE6 / 255)
    /// emerald is the success and action color.
    static let emerald = Color(red: 0x2F / 255, green: 0xBF / 255, blue: 0x8F / 255)
    /// ember marks failures.
    static let ember = Color(red: 0xF0 / 255, green: 0x7A / 255, blue: 0x6A / 255)

    /// brand runs across the logo colors.
    static let brand = LinearGradient(colors: [bone, ice, emerald], startPoint: .leading, endPoint: .trailing)

    /// background is the window backdrop with a soft ice glow from the top.
    static var background: some View {
        ZStack {
            midnight
            RadialGradient(colors: [ice.opacity(0.14), .clear], center: .top, startRadius: 0, endRadius: 520)
            RadialGradient(colors: [emerald.opacity(0.08), .clear], center: .bottomTrailing,
                           startRadius: 0, endRadius: 480)
        }
        .ignoresSafeArea()
    }

    /// surface is the fill for cards and rows.
    static func surface(selected: Bool = false) -> some ShapeStyle {
        selected ? AnyShapeStyle(ice.opacity(0.16)) : AnyShapeStyle(Color.white.opacity(0.045))
    }
}

/// GradientBar is a slim progress bar filled with the brand gradient.
struct GradientBar: View {
    /// value is the fill from 0 to 1.
    var value: Double

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Capsule().fill(Color.white.opacity(0.08))
                Capsule().fill(Theme.brand)
                    .frame(width: max(6, geo.size.width * min(1, max(0, value))))
                    .animation(.easeOut(duration: 0.35), value: value)
            }
        }
        .frame(height: 6)
        .accessibilityElement()
        .accessibilityValue(Text("\(Int(value * 100)) percent"))
    }
}
