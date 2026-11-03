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
    /// ext is the file extension written, without the dot.
    public var ext: String
    /// detail describes the target for the hover text.
    public var detail: String
    /// available is false when this ffmpeg build cannot write the target.
    public var available: Bool
}

/// TargetSection is one titled group of targets in the sidebar.
public struct TargetSection: Identifiable, Hashable, Sendable {
    /// Kind names a sidebar section.
    public enum Kind: String, CaseIterable, Sendable {
        /// recent holds the presets used last.
        case recent
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
            case .recent: "Recent"
            case .presets: "Presets"
            case .video: "Video"
            case .audio: "Audio"
            case .custom: "Custom"
            }
        }
    }

    /// kind names the section.
    public var kind: Kind
    /// targets are the rows in display order.
    public var targets: [Target]

    /// id identifies the section.
    public var id: Kind { kind }
    /// title is the section heading.
    public var title: String { kind.title }
}

/// Targets builds the sidebar sections from the presets and the catalog.
public enum Targets {
    /// recentLimit is the most presets the Recent section lists.
    public static let recentLimit = 5
    /// unavailableHelp is the hover text on formats this ffmpeg build cannot write.
    public static let unavailableHelp = "Needs the bundled ffmpeg."

    /// sections returns Recent, Presets, Video, Audio, and Custom in that order, each filtered by
    /// search. A blank search keeps every row. Sections may be empty.
    public static func sections(presets: [Preset], recent: [String], catalog: Catalog?,
                                search: String = "") -> [TargetSection] {
        let byID = Dictionary(presets.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        let recents = recent.compactMap { byID[$0] }.prefix(recentLimit)
        let all: [TargetSection.Kind: [Target]] = [
            .recent: recents.map { target(for: $0, catalog: catalog, prefix: "recent") },
            .presets: presets.filter { !$0.isCustom }.map { target(for: $0, catalog: catalog, prefix: "preset") },
            .video: catalog.map { cat in cat.videoContainers.map { target(for: $0, catalog: cat) } } ?? [],
            .audio: catalog.map { cat in cat.audioContainers.map { target(for: $0, catalog: cat) } } ?? [],
            .custom: presets.filter(\.isCustom).map { target(for: $0, catalog: catalog, prefix: "preset") },
        ]
        let query = search.trimmingCharacters(in: .whitespacesAndNewlines)
        return TargetSection.Kind.allCases.map { kind in
            let rows = all[kind] ?? []
            return TargetSection(kind: kind, targets: query.isEmpty ? rows : rows.filter { matches($0, query) })
        }
    }

    /// selectedID returns the row ID of the target the settings came from: the preset when one is
    /// selected, else the catalog format.
    public static func selectedID(presetID: String, container: String?) -> String? {
        if !presetID.isEmpty { return "preset:" + presetID }
        return container.map { "container:" + $0 }
    }

    /// matches reports whether a target's name, extension, or detail contains the query, ignoring
    /// case and diacritics.
    static func matches(_ t: Target, _ query: String) -> Bool {
        [t.name, t.ext, t.detail].contains { $0.localizedStandardContains(query) }
    }

    /// target makes the row for a preset.
    static func target(for p: Preset, catalog: Catalog?, prefix: String) -> Target {
        Target(id: prefix + ":" + p.id, pick: .preset(p.id), name: p.name,
               symbol: p.isCustom ? "slider.horizontal.3" : symbol(forPreset: p.id), ext: p.ext,
               detail: p.description, available: true)
    }

    /// target makes the row for a catalog container.
    static func target(for c: Container, catalog: Catalog) -> Target {
        let usable = catalog.isUsable(c)
        return Target(id: "container:" + c.id, pick: .container(c.id), name: c.name,
                      symbol: c.kind == "video" ? "film" : "waveform", ext: c.ext,
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
}
