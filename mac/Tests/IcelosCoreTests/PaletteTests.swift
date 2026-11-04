import Foundation
import Testing

@testable import IcelosCore

/// everyPalette lists each theme in each appearance it draws, labeled for failure messages.
let everyPalette: [(String, Palette)] = ThemeName.allCases.flatMap { theme in
    theme.followsAppearance
        ? [("\(theme.name) dark", theme.palette(dark: true)), ("\(theme.name) light", theme.palette(dark: false))]
        : [(theme.name, theme.palette(dark: true))]
}

@Test func swatchHex() {
    #expect(Swatch(hex: "#C9A24B") == Swatch(red: 0xC9 / 255.0, green: 0xA2 / 255.0, blue: 0x4B / 255.0))
    #expect(Swatch(hex: "0b0b0b") == Swatch(hex: "#0B0B0B"))
    #expect(Swatch(hex: "#C9A24") == nil)
    #expect(Swatch(hex: "#GGGGGG") == nil)
    #expect(Swatch(hex: "") == nil)
}

@Test func swatchContrast() {
    #expect(abs(Swatch.black.contrast(with: .white) - 21) < 0.001)
    #expect(Swatch.white.contrast(with: .white) == 1)
    let gold = Swatch(hex: "#C9A24B")!
    let obsidian = Swatch(hex: "#0B0B0B")!
    #expect(abs(gold.contrast(with: obsidian) - obsidian.contrast(with: gold)) < 1e-12)
    #expect(abs(gold.contrast(with: obsidian) - 8.2) < 0.05)
}

/// ContrastRule is one row of the design doc's contrast table.
struct ContrastRule: CustomTestStringConvertible, Sendable {
    /// fore is the slot drawn on top.
    var fore: Palette.Slot
    /// back is the slot drawn under it.
    var back: Palette.Slot
    /// minimum is the lowest ratio that passes.
    var minimum: Double

    var testDescription: String { "\(fore) on \(back) at \(minimum)" }
}

/// contrastRules is the contrast table from the design doc.
let contrastRules: [ContrastRule] = [
    ContrastRule(fore: .text, back: .background, minimum: 4.5),
    ContrastRule(fore: .text, back: .surface, minimum: 4.5),
    ContrastRule(fore: .muted, back: .background, minimum: 4.5),
    ContrastRule(fore: .muted, back: .surface, minimum: 4.5),
    ContrastRule(fore: .accentText, back: .background, minimum: 4.5),
    ContrastRule(fore: .accentText, back: .surface, minimum: 4.5),
    ContrastRule(fore: .accent, back: .background, minimum: 3),
    ContrastRule(fore: .onAccent, back: .accent, minimum: 4.5),
    ContrastRule(fore: .success, back: .background, minimum: 4.5),
    ContrastRule(fore: .danger, back: .background, minimum: 4.5),
]

@Test(arguments: contrastRules) func everyThemePasses(rule: ContrastRule) {
    for (label, palette) in everyPalette {
        let ratio = palette.swatch(rule.fore).contrast(with: palette.swatch(rule.back))
        #expect(ratio >= rule.minimum, "\(label): \(rule.fore) on \(rule.back) is \(ratio)")
    }
}

@Test func onAccentRules() {
    let obsidian = ThemeName.obsidianGold.palette(dark: true)
    #expect(obsidian.onAccent == obsidian.background)
    let redcoat = ThemeName.redcoat.palette(dark: true)
    #expect(redcoat.onAccent == .white)
    let harborLight = ThemeName.harbor.palette(dark: false)
    #expect(harborLight.onAccent == .black)
}

@Test func accentTextRules() {
    let obsidian = ThemeName.obsidianGold.palette(dark: true)
    #expect(obsidian.accentText == obsidian.accent)
    let redcoat = ThemeName.redcoat.palette(dark: true)
    #expect(redcoat.accentText == redcoat.text)
}

@Test func designDocTable() {
    let dark = ThemeName.obsidianGold.palette(dark: true)
    #expect(dark.background == Swatch(hex: "#0B0B0B"))
    #expect(dark.raised == Swatch(hex: "#1E1E21"))
    #expect(dark.accent == Swatch(hex: "#C9A24B"))
    #expect(dark.border == Swatch(hex: "#F7F4E9")!.opacity(0.10))
    #expect(dark.isDark)
    let light = ThemeName.obsidianGold.palette(dark: false)
    #expect(light.background == Swatch(hex: "#F6F3EC"))
    #expect(light.accent == Swatch(hex: "#8A6A1E"))
    #expect(light.success == Swatch(hex: "#1F7A4D"))
    #expect(light.border == Swatch(hex: "#1A1916")!.opacity(0.12))
    #expect(!light.isDark)
}

@Test func derivedRaised() {
    let harbor = ThemeName.harbor.palette(dark: true)
    #expect(harbor.raised != harbor.surface)
    #expect(harbor.raised.luminance > harbor.surface.luminance)
    let bletchley = ThemeName.bletchley.palette(dark: true)
    #expect(bletchley.raised.luminance < bletchley.surface.luminance)
}

@Test func themeNames() {
    #expect(ThemeName.allCases.count == 12)
    #expect(Set(ThemeName.allCases.map(\.name)).count == 12)
    #expect(ThemeName(stored: nil) == .obsidianGold)
    #expect(ThemeName(stored: "icelos") == .obsidianGold)
    #expect(ThemeName(stored: "redcoat") == .redcoat)
    #expect(ThemeName.allCases.filter(\.followsAppearance) == [.obsidianGold, .harbor])
}

@Test func appearanceDecidesDarkness() {
    #expect(ThemeName.obsidianGold.forcedDark(.system) == nil)
    #expect(ThemeName.obsidianGold.forcedDark(.light) == false)
    #expect(ThemeName.harbor.forcedDark(.dark) == true)
    #expect(ThemeName.phosphor.forcedDark(.light) == true)
    #expect(ThemeName.phosphor.forcedDark(.system) == true)
    #expect(ThemeName.bletchley.forcedDark(.dark) == false)
}

/// IncreasedContrastTests covers the palette the system's increased contrast setting selects.
@Suite struct IncreasedContrastTests {
    @Test func raisesMutedAndBorder() {
        for (label, palette) in everyPalette {
            let high = palette.increasedContrast
            #expect(high.muted == palette.text, "\(label)")
            #expect(high.border.alpha == 0.35, "\(label)")
            #expect(high.border.opacity(1) == palette.text, "\(label)")
            #expect(high.background == palette.background && high.accent == palette.accent, "\(label)")
        }
    }
}

/// bundleScript is the text of mac/bundle.sh, found from this file's path.
private func bundleScript() throws -> String {
    let url = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        .appendingPathComponent("bundle.sh")
    return try String(contentsOf: url, encoding: .utf8)
}

/// hexComponents lists every quoted "0xNN" component in the text, in the order written.
private func hexComponents(in text: String) -> [String] {
    var found: [String] = []
    var rest = Substring(text)
    while let start = rest.range(of: "\"0x") {
        let digits = rest[start.upperBound...].prefix(2)
        let afterDigits = rest[start.upperBound...].dropFirst(2)
        if digits.count == 2, digits.allSatisfy(\.isHexDigit), afterDigits.first == "\"" {
            found.append("0x" + digits.uppercased())
        }
        rest = rest[start.upperBound...]
    }
    return found
}

/// assetComponents returns a swatch's red, green, and blue as the "0xNN" strings bundle.sh writes.
private func assetComponents(_ swatch: Swatch) -> [String] {
    [swatch.red, swatch.green, swatch.blue].map { String(format: "0x%02X", Int(($0 * 255).rounded())) }
}

@Test func systemAccentValues() {
    #expect(Palette.systemAccent(dark: false) == Swatch(hex: "#8A6A1E"))
    #expect(Palette.systemAccent(dark: true) == Swatch(hex: "#B08A32"))
}

@Test func systemAccentContrast() {
    let dark = Palette.systemAccent(dark: true)
    let light = Palette.systemAccent(dark: false)
    #expect(Swatch.white.contrast(with: dark) >= 3.0, "dark accent with white is \(Swatch.white.contrast(with: dark))")
    #expect(Swatch.white.contrast(with: light) >= 4.5, "light accent with white is \(Swatch.white.contrast(with: light))")
}

@Test func bundleScriptCarriesSystemAccent() throws {
    let expected = assetComponents(Palette.systemAccent(dark: false)) + assetComponents(Palette.systemAccent(dark: true))
    let written = hexComponents(in: try bundleScript())
    let carried = written.indices.contains { start in
        written.dropFirst(start).prefix(expected.count).elementsEqual(expected)
    }
    #expect(carried, "bundle.sh components \(written) do not hold \(expected) in order")
}
