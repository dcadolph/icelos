import Foundation

/// Target is one output choice in the sidebar: a preset or a catalog format.
public struct Target: Identifiable, Hashable, Sendable {
    /// Pick is what choosing a target selects.
    public enum Pick: Hashable, Sendable {
        /// preset selects the built-in or custom preset with the ID.
        case preset(String)
        /// container selects the catalog's starting settings for the container with the ID.
        case container(String)
    }

    /// id identifies the row, unique across sections.
    public var id: String
    /// pick is what choosing the row selects.
    public var pick: Pick
    /// name is the row label.
    public var name: String
    /// symbol is the SF Symbol shown before the name.
    public var symbol: String
    /// detail describes the target for the hover text.
    public var detail: String
    /// available is false when this ffmpeg build cannot write the target.
    public var available: Bool
}

/// TargetSection is one titled group of targets in the sidebar.
public struct TargetSection: Identifiable, Hashable, Sendable {
    /// Kind names a sidebar section.
    public enum Kind: String, CaseIterable, Sendable {
        /// presets holds the built-in presets.
        case presets
        /// video holds the video formats from the catalog.
        case video
        /// audio holds the audio formats from the catalog.
        case audio
        /// custom holds the presets the user saved.
        case custom

        /// title is the section heading.
        public var title: String {
            switch self {
            case .presets: "Presets"
            case .video: "Video"
            case .audio: "Audio"
            case .custom: "Custom"
            }
        }
    }

    /// kind names the section.
    public var kind: Kind
    /// targets are the rows listed at the top level, in display order.
    public var targets: [Target]
    /// more are the rows folded under More Formats, empty for Presets and Custom.
    public var more: [Target] = []

    /// id identifies the section.
    public var id: Kind { kind }
    /// title is the section heading.
    public var title: String { kind.title }
    /// isEmpty reports whether the section has no rows at either level.
    public var isEmpty: Bool { targets.isEmpty && more.isEmpty }
}

/// Targets builds the sidebar sections from the presets and the catalog.
public enum Targets {
    /// unavailableHelp is the hover text on formats this ffmpeg build cannot write.
    public static let unavailableHelp = "Needs the bundled ffmpeg."
    /// customSymbol is the symbol on every saved preset.
    static let customSymbol = "slider.horizontal.3"

    /// sections returns Presets, Video, Audio, and Custom in that order, each filtered by search.
    /// Video and Audio list the common formats and fold the rest into more. A blank search keeps
    /// every row. Sections may be empty.
    public static func sections(presets: [Preset], catalog: Catalog?, search: String = "") -> [TargetSection] {
        let query = search.trimmingCharacters(in: .whitespacesAndNewlines)
        let keep = { (rows: [Target]) in query.isEmpty ? rows : rows.filter { matches($0, query) } }
        let rows = { (cs: [Container], cat: Catalog) in cs.map { target(for: $0, catalog: cat) } }
        return TargetSection.Kind.allCases.map { kind in
            switch kind {
            case .presets:
                TargetSection(kind: kind, targets: keep(presets.filter { !$0.isCustom }.map(target(for:))))
            case .video:
                TargetSection(kind: kind, targets: keep(catalog.map { rows($0.commonVideoContainers, $0) } ?? []),
                              more: keep(catalog.map { rows($0.moreVideoContainers, $0) } ?? []))
            case .audio:
                TargetSection(kind: kind, targets: keep(catalog.map { rows($0.commonAudioContainers, $0) } ?? []),
                              more: keep(catalog.map { rows($0.moreAudioContainers, $0) } ?? []))
            case .custom:
                TargetSection(kind: kind, targets: keep(presets.filter(\.isCustom).map(target(for:))))
            }
        }
    }

    /// selectedID returns the row ID of the target the settings came from: the preset when one is
    /// selected, else the catalog format.
    public static func selectedID(presetID: String, container: String?) -> String? {
        if !presetID.isEmpty { return "preset:" + presetID }
        return container.map { "container:" + $0 }
    }

    /// matches reports whether a target's name or detail contains the query, ignoring case and
    /// diacritics.
    static func matches(_ t: Target, _ query: String) -> Bool {
        [t.name, t.detail].contains { $0.localizedStandardContains(query) }
    }

    /// target makes the row for a preset.
    static func target(for p: Preset) -> Target {
        Target(id: "preset:" + p.id, pick: .preset(p.id), name: p.name,
               symbol: p.isCustom ? customSymbol : symbol(forPreset: p.id), detail: p.description,
               available: true)
    }

    /// target makes the row for a catalog container.
    static func target(for c: Container, catalog: Catalog) -> Target {
        let usable = catalog.isUsable(c)
        return Target(id: "container:" + c.id, pick: .container(c.id), name: c.name,
                      symbol: symbol(forContainer: c.id, kind: c.kind),
                      detail: usable ? catalog.summary(catalog.target(for: c)) : unavailableHelp,
                      available: usable)
    }

    /// symbol picks an SF Symbol for a built-in preset ID.
    public static func symbol(forPreset id: String) -> String {
        switch id {
        case "apple": "apple.logo"
        case "compatible": "play.tv"
        case "web": "globe"
        case "tiny": "paperplane"
        case "target-size": "scalemass"
        case "remux": "shippingbox"
        case "audio": "waveform"
        case "mp3": "music.note"
        default: "film"
        }
    }

    /// symbol picks an SF Symbol for a container by the family it belongs to, so peers share one,
    /// falling back to film or waveform by kind.
    public static func symbol(forContainer id: String, kind: String) -> String {
        switch id {
        case "mp4", "mov", "m4v": "film"
        case "mkv", "ogv": "film.stack"
        case "webm": "globe"
        case "avi", "wmv", "asf": "pc"
        case "mpg", "vob", "m2ts": "opticaldisc"
        case "ts", "trp": "antenna.radiowaves.left.and.right"
        case "3gp", "3g2", "amr": "phone"
        case "mxf": "movieclapper"
        case "dv": "video"
        case "flv", "f4v", "swf", "amv": "play.rectangle"
        case "mp3", "m4a", "aac", "ogg", "opus", "wma", "mp2", "mka": "music.note"
        case "wav", "aiff", "au", "caf", "flac", "alac": "waveform"
        case "m4r": "bell"
        case "m4b": "book.closed"
        case "ac3": "hifispeaker"
        default: kind == "audio" ? "waveform" : "film"
        }
    }

    /// allSymbols lists every symbol the sidebar can draw for these presets and this catalog,
    /// including the fallbacks, sorted and without repeats.
    public static func allSymbols(presets: [Preset], catalog: Catalog?) -> [String] {
        var out = Set(presets.map { target(for: $0).symbol })
        out.formUnion([customSymbol, symbol(forPreset: ""), symbol(forContainer: "", kind: "video"),
                       symbol(forContainer: "", kind: "audio")])
        for c in catalog?.containers ?? [] { out.insert(symbol(forContainer: c.id, kind: c.kind)) }
        return out.sorted()
    }
}
