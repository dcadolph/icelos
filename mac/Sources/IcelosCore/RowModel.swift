import Foundation

/// RowModel is everything a file row shows, worked out from the item and the settings it will
/// use, so the view draws it without logic of its own.
public struct RowModel: Equatable, Sendable {
    /// Action is a trailing row button.
    public enum Action: String, Equatable, Sendable {
        /// cancel stops a queued or running file.
        case cancel
        /// reveal shows the output in Finder.
        case reveal
        /// retry puts a failed or canceled file back to pending.
        case retry
        /// remove drops the file from the list.
        case remove

        /// label is the button's help text and VoiceOver name.
        public var label: String {
            switch self {
            case .cancel: "Cancel"
            case .reveal: "Show in Finder"
            case .retry: "Try again"
            case .remove: "Remove from list"
            }
        }

        /// symbol is the SF Symbol drawn on the button.
        public var symbol: String {
            switch self {
            case .cancel: "stop.circle"
            case .reveal: "magnifyingglass.circle"
            case .retry: "arrow.clockwise.circle"
            case .remove: "xmark.circle"
            }
        }
    }

    /// Badge is the state label with its palette color.
    public struct Badge: Hashable, Sendable {
        /// text is the label.
        public var text: String
        /// tone is the palette slot it is drawn in.
        public var tone: Palette.Slot
    }

    /// Media is what the thumbnail tile shows when no frame is cached.
    public enum Media: Equatable, Sendable {
        /// video is a file with motion video.
        case video
        /// audio is a file with sound only, shown as a waveform tile.
        case audio
        /// unknown is a file not read yet or unreadable.
        case unknown
    }

    /// name is the source file name.
    public var name: String
    /// source describes the source file, or why it cannot be read.
    public var source: String
    /// sourceTone colors the source line.
    public var sourceTone: Palette.Slot
    /// chips name the output: extension, frame size, and codec, or codec and bitrate for audio.
    public var chips: [String]
    /// ownSettings is true when the file uses its own settings rather than the window's.
    public var ownSettings: Bool
    /// estimate is the expected size before converting, e.g. "≈ 48 MB, 71% smaller", or a range
    /// such as "≈ 8 to 25 MB, at least 40% smaller" in quality mode.
    public var estimate: String?
    /// detail is the progress, result, or failure line, which replaces the chips once the file
    /// runs or finishes.
    public var detail: String?
    /// detailTone colors the detail line.
    public var detailTone: Palette.Slot
    /// progress is the completion from 0 to 1 while running.
    public var progress: Double?
    /// badge is the state label.
    public var badge: Badge
    /// actions are the trailing buttons in order.
    public var actions: [Action]
    /// media picks the thumbnail tile.
    public var media: Media

    /// Creates the row for item, which converts with its own settings or else windowSettings.
    /// TargetSize is the size typed for presets that aim for one, and hardware is true when the
    /// media engine is on.
    public init(item: Item, windowSettings: Preset?, catalog: Catalog?, targetSize: String = "",
                hardware: Bool = false) {
        let settings = item.settings ?? windowSettings
        name = item.url.lastPathComponent
        ownSettings = item.settings != nil
        media = item.info.map { $0.video == nil ? .audio : .video } ?? .unknown
        if let error = item.probeError {
            source = error
            sourceTone = .danger
        } else {
            source = item.info.map(Format.summary) ?? "Reading"
            sourceTone = .muted
        }
        chips = settings.map { RowModel.chips($0, info: item.info, catalog: catalog) } ?? []
        estimate = nil
        detail = nil
        detailTone = .muted
        progress = nil
        badge = RowModel.badge(item)
        actions = RowModel.actions(item)

        switch item.phase {
        case .pending, .queued:
            if let info = item.info, let settings,
               let range = Estimate.bytes(info, settings, targetBytes: Estimate.parseSize(targetSize),
                                          hardware: hardware) {
                estimate = Estimate.text(range, source: info.size > 0 ? info.size : nil)
            }
        case .running:
            progress = item.status?.progress.percent ?? 0
            detail = RowModel.running(item.status?.progress)
        case .done:
            detail = RowModel.done(item)
            detailTone = .success
        case .failed:
            detail = item.errorMessage ?? "Failed."
            detailTone = .danger
        case .canceled:
            break
        }
    }

    /// badge names the item's state. A file not read yet, or unreadable, gets its own label.
    static func badge(_ item: Item) -> Badge {
        switch item.phase {
        case .pending:
            if item.probeError != nil { return Badge(text: "Unreadable", tone: .danger) }
            return item.info == nil ? Badge(text: "Reading", tone: .muted) : Badge(text: "Ready", tone: .muted)
        case .queued:
            return Badge(text: "Queued", tone: .muted)
        case .running:
            let percent = Int(((item.status?.progress.percent ?? 0) * 100).rounded(.down))
            return Badge(text: "\(percent)%", tone: .accent)
        case .done:
            return Badge(text: "Done", tone: .success)
        case .failed:
            return Badge(text: "Failed", tone: .danger)
        case .canceled:
            return Badge(text: "Canceled", tone: .muted)
        }
    }

    /// actions lists the buttons for the item's state.
    static func actions(_ item: Item) -> [Action] {
        switch item.phase {
        case .queued, .running: [.cancel]
        case .done: [.reveal, .remove]
        case .failed, .canceled: item.probeError == nil ? [.retry, .remove] : [.remove]
        case .pending: [.remove]
        }
    }

    /// chips names the output of p for a source: the extension, then the frame size and video
    /// codec, or the audio codec and bitrate when the output has no video.
    static func chips(_ p: Preset, info: MediaInfo?, catalog: Catalog?) -> [String] {
        var out = [p.ext.uppercased()]
        func name(_ id: String) -> String {
            id == copyCodec ? "Copy" : (catalog?.codec(id: id)?.name ?? id.uppercased())
        }
        let hasVideo = !p.video.codec.isEmpty && (info.map { $0.video != nil } ?? true)
        if hasVideo {
            if let src = info?.video, let (w, h) = Estimate.frame(src, p.video) { out.append("\(w)×\(h)") }
            out.append(name(p.video.codec))
        } else if !p.audio.codec.isEmpty {
            out.append(name(p.audio.codec))
            if let rate = p.audio.bitrate, p.audio.codec != copyCodec { out.append("\(rate / 1000) kb/s") }
        }
        return out
    }

    /// running describes live progress after the percent, which the badge shows.
    static func running(_ p: JobProgress?) -> String {
        guard let p, p.percent > 0 || p.speed > 0 else { return "Starting" }
        var parts: [String] = []
        if p.passes > 1 { parts.append("Pass \(p.pass) of \(p.passes)") }
        if p.speed > 0 { parts.append(String(format: "%.1f×", p.speed)) }
        if p.etaNs > 0 { parts.append(Format.duration(ns: p.etaNs) + " left") }
        if let enc = Format.encoder(p) { parts.append(enc) }
        return parts.isEmpty ? "Starting" : parts.joined(separator: " · ")
    }

    /// done describes the finished output: its name, size, and the change from the source.
    static func done(_ item: Item) -> String {
        var parts = [item.outputURL?.lastPathComponent ?? "Done"]
        if let out = item.outputSize {
            parts.append(Format.bytes(out))
            if let src = item.info?.size, let saving = Format.saving(from: src, to: out) { parts.append(saving) }
        }
        return parts.joined(separator: " · ")
    }
}
