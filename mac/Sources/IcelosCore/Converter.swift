import CoreGraphics
import Foundation
import Observation

/// Phase is where a file is from the app's point of view.
public enum Phase: Sendable, Equatable {
    /// pending has not been sent to the converter.
    case pending
    /// queued waits for a worker.
    case queued
    /// running is encoding.
    case running
    /// done has its output in place.
    case done
    /// failed stopped with an error.
    case failed
    /// canceled was stopped before it finished.
    case canceled

    /// isFinished is true for final phases.
    public var isFinished: Bool { self == .done || self == .failed || self == .canceled }
}

/// Item is one file in the window.
public struct Item: Identifiable, Sendable, Equatable {
    /// id identifies the item for the lifetime of the window.
    public let id: UUID
    /// url is the source file.
    public let url: URL
    /// info is the probe result once it arrives.
    public var info: MediaInfo?
    /// probeError is set when the file could not be read.
    public var probeError: String?
    /// submitted is true once the add request is sent.
    public var submitted = false
    /// jobID is the converter's job identifier.
    public var jobID: String?
    /// status is the latest job snapshot.
    public var status: JobStatus?
    /// addError is set when the converter refused the job.
    public var addError: String?
    /// outputSize is the finished file size.
    public var outputSize: Int64?
    /// settings overrides the window settings for this file when set.
    public var settings: Preset?

    /// phase summarizes the item state.
    public var phase: Phase {
        if let status {
            switch status.state {
            case .queued: return .queued
            case .running: return .running
            case .done: return .done
            case .failed: return .failed
            case .canceled: return .canceled
            }
        }
        if addError != nil { return .failed }
        return submitted ? .queued : .pending
    }

    /// errorMessage is the most relevant failure text.
    public var errorMessage: String? { status?.error ?? addError ?? probeError }

    /// outputURL is the destination once known.
    public var outputURL: URL? { status.map { URL(fileURLWithPath: $0.job.output) } }
}

/// Converter is the window model. It turns user actions into requests and backend messages
/// into item state.
@MainActor
@Observable
public final class Converter {
    /// presets are the built-in presets in display order, then the custom ones.
    public private(set) var presets: [Preset] = []
    /// catalog is the format catalog, once the backend sends it.
    public private(set) var catalog: Catalog?
    /// items are the files in the window in the order they were added.
    public private(set) var items: [Item] = []
    /// encoders lists the usable ffmpeg encoders.
    public private(set) var encoders: [String] = []
    /// version is the converter version.
    public private(set) var version: String?
    /// fault is a problem with the converter itself.
    public private(set) var fault: String?
    /// presetID is the selected built-in or custom preset, or empty when the settings come
    /// straight from the catalog. Selecting a preset replaces the settings with a copy of it.
    public var presetID = "apple" {
        didSet { if let p = preset(id: presetID) { settings = p } }
    }
    /// settings is the editable copy of what the next Convert produces.
    public var settings: Preset?
    /// presetError is the latest problem saving, deleting, or reading custom presets.
    public private(set) var presetError: String?
    /// recent lists the preset IDs used last, newest first.
    public private(set) var recent: [String] = []
    /// selection is the selected file, whose override the inspector edits when it has one.
    public var selection: UUID?
    /// hardware prefers the media engine.
    public var hardware = true
    /// targetSize is the size for the target-size preset.
    public var targetSize = "25MB"
    /// outputFolder is where outputs go, or nil for next to each source.
    public var outputFolder: URL?
    /// thumbnails holds the row frame for each source with motion video, once it is made.
    public private(set) var thumbnails: [URL: CGImage] = [:]

    /// send delivers a request to the backend.
    @ObservationIgnored private let send: (Request) -> Void
    /// savingID is the ID of the custom preset being saved, selected once the save lands.
    @ObservationIgnored private var savingID: String?
    /// early holds job events that arrived before their add reply.
    @ObservationIgnored private var early: [String: JobStatus] = [:]
    /// fileSize reads a file size, injectable for tests.
    @ObservationIgnored private let fileSize: (URL) -> Int64?
    /// thumbnail makes a row frame for a source, injectable for tests.
    @ObservationIgnored private let thumbnail: @Sendable (URL) async -> CGImage?
    /// thumbnailTasks are the frames asked for, one per source, kept so none is asked twice.
    @ObservationIgnored var thumbnailTasks: [URL: Task<Void, Never>] = [:]

    /// Creates a converter that sends requests through send.
    public init(send: @escaping (Request) -> Void,
                fileSize: @escaping (URL) -> Int64? = Converter.diskSize,
                thumbnail: @escaping @Sendable (URL) async -> CGImage? = { await Thumbnails.image(for: $0) }) {
        self.send = send
        self.fileSize = fileSize
        self.thumbnail = thumbnail
    }

    /// diskSize returns the size of the file at url.
    public nonisolated static func diskSize(_ url: URL) -> Int64? {
        (try? FileManager.default.attributesOfItem(atPath: url.path)[.size] as? NSNumber)?.int64Value
    }

    /// preset returns the built-in or custom preset with id.
    public func preset(id: String) -> Preset? { presets.first { $0.id == id } }

    /// builtinPresets lists the built-in presets.
    public var builtinPresets: [Preset] { presets.filter { !$0.isCustom } }

    /// customPresets lists the presets the user saved.
    public var customPresets: [Preset] { presets.filter(\.isCustom) }

    /// container is the catalog container of the settings.
    public var container: Container? { catalog?.container(id: settings?.container) }

    /// isEdited is true when the settings differ from the selected preset, or no preset is
    /// selected.
    public var isEdited: Bool {
        guard let settings else { return false }
        return preset(id: presetID) != settings
    }

    /// selectedTargetID is the sidebar row the settings came from.
    public var selectedTargetID: String? {
        Targets.selectedID(presetID: presetID, container: settings?.container)
    }

    /// targetSections are the sidebar sections filtered by search.
    public func targetSections(search: String = "") -> [TargetSection] {
        Targets.sections(presets: presets, recent: recent, catalog: catalog, search: search)
    }

    /// choose makes a sidebar target the window settings. Unavailable formats are ignored.
    public func choose(_ pick: Target.Pick) {
        switch pick {
        case .preset(let id):
            guard preset(id: id) != nil else { return }
            if presetID == id, let p = preset(id: id) { settings = p } else { presetID = id }
        case .container(let id):
            guard let catalog, let c = catalog.container(id: id), catalog.isUsable(c) else { return }
            select(container: c)
        }
    }

    /// inspectorScope is what the inspector edits: the window settings, or the selected pending
    /// file, which either follows the window settings or has its own.
    public var inspectorScope: InspectorScope {
        guard let id = selection, let item = items.first(where: { $0.id == id }), item.phase == .pending,
              item.probeError == nil else { return .window }
        return .item(id, name: item.url.lastPathComponent, usesWindow: item.settings == nil)
    }

    /// inspected is the settings the inspector shows.
    public var inspected: Preset? {
        if case .item(let id, _, false) = inspectorScope {
            return items.first { $0.id == id }?.settings
        }
        return settings
    }

    /// inspectedOriginal is what the inspector's Reset goes back to: the window settings for a
    /// file's own settings, else the selected preset or the format's starting settings.
    public var inspectedOriginal: Preset? {
        if case .item(_, _, false) = inspectorScope { return settings }
        guard let base = settings else { return nil }
        if let p = preset(id: presetID) { return p }
        guard let catalog, let c = catalog.container(id: base.container) else { return base }
        return catalog.target(for: c)
    }

    /// editInspected applies an inspector edit to the selected file's own settings when it has
    /// them, else to the window settings. A file's own settings stay its own even when they come
    /// to match the window settings, so the switch does not flip under the user.
    public func editInspected(_ p: Preset) {
        if case .item(let id, _, false) = inspectorScope {
            update(id) { $0.settings = p }
        } else {
            settings = p
        }
    }

    /// setUsesWindowSettings makes a pending file follow the window settings, or gives it its own
    /// copy of them to edit.
    public func setUsesWindowSettings(_ id: UUID, _ on: Bool) {
        let base = settings
        update(id) { item in
            guard item.phase == .pending else { return }
            item.settings = on ? nil : (item.settings ?? base)
        }
    }

    /// hasHardware is true when a VideoToolbox encoder is available.
    public var hasHardware: Bool { encoders.contains { $0.hasSuffix("_videotoolbox") } }

    /// pendingCount is how many items wait for Convert.
    public var pendingCount: Int { items.filter { $0.phase == .pending && $0.probeError == nil }.count }

    /// isBusy is true while any item is queued or running.
    public var isBusy: Bool { items.contains { $0.phase == .queued || $0.phase == .running } }

    /// hasFinished is true when any item is finished.
    public var hasFinished: Bool { items.contains { $0.phase.isFinished } }

    /// savedBytes is the total size reduction over finished items.
    public var savedBytes: Int64 {
        items.reduce(0) { sum, item in
            guard item.phase == .done, let out = item.outputSize, let src = item.info?.size else { return sum }
            return sum + (src - out)
        }
    }

    /// overallProgress is the mean progress over submitted items, or nil when none run.
    public var overallProgress: Double? {
        let active = items.filter { $0.submitted }
        guard active.contains(where: { $0.phase == .queued || $0.phase == .running }) else { return nil }
        let total = active.reduce(0.0) { $0 + ($1.phase.isFinished ? 1 : ($1.status?.progress.percent ?? 0)) }
        return total / Double(active.count)
    }

    /// add appends files and asks the converter to probe each one. Duplicates of a pending
    /// file are skipped.
    public func add(urls: [URL]) {
        for url in urls {
            if items.contains(where: { $0.url == url && $0.phase == .pending }) { continue }
            let item = Item(id: UUID(), url: url)
            items.append(item)
            send(Request(id: "probe:" + item.id.uuidString, op: "probe", path: url.path))
        }
    }

    /// convertAll submits every pending, readable item. An unedited preset goes by ID, so the
    /// backend records it as recently used; edited settings and per-file overrides go inline.
    public func convertAll() {
        guard let base = settings else { return }
        for i in items.indices where items[i].phase == .pending && items[i].probeError == nil {
            items[i].submitted = true
            let p = items[i].settings ?? base
            let byID = items[i].settings == nil && !isEdited
            send(Request(id: "add:" + items[i].id.uuidString, op: "add", input: items[i].url.path,
                         preset: byID ? p.id : nil, presetBody: byID ? nil : p,
                         size: p.needsSize ? targetSize : nil, hardware: hardware,
                         outDir: outputFolder?.path))
        }
    }

    /// select makes the catalog's starting settings for container c, at a resolution, the window
    /// settings. Picking a resolution on the container already selected keeps the other edits.
    public func select(container c: Container, resolution: Resolution? = nil) {
        guard let catalog else { return }
        if presetID.isEmpty, let current = settings, current.container == c.id {
            if let resolution { settings = catalog.fitting(current, into: resolution) }
            return
        }
        presetID = ""
        settings = catalog.target(for: c, resolution: resolution)
    }

    /// setContainer moves the window settings into the container with id, resetting the codecs
    /// to the container's first available ones.
    public func setContainer(_ id: String) {
        guard let catalog, let c = catalog.container(id: id), let current = settings else { return }
        settings = catalog.retarget(current, to: c)
    }

    /// override sets the settings for one pending file. Settings equal to the window settings, or
    /// nil, clear the override.
    public func override(_ id: UUID, with preset: Preset?) {
        update(id) { item in
            guard item.phase == .pending else { return }
            item.settings = preset == settings ? nil : preset
        }
    }

    /// savePreset asks the backend to save p as a custom preset named name. The new preset is
    /// selected once the save lands.
    public func savePreset(_ p: Preset, name: String) {
        var body = p
        body.id = Converter.presetID(for: name, taken: builtinPresets.map(\.id))
        body.name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        body.description = catalog?.summary(p) ?? p.description
        body.custom = nil
        body.targetBytes = nil
        savingID = body.id
        presetError = nil
        send(Request(id: "preset.save", op: "preset.save", presetBody: body))
    }

    /// deletePreset asks the backend to delete the custom preset with id.
    public func deletePreset(_ id: String) {
        presetError = nil
        send(Request(id: "preset.delete", op: "preset.delete", preset: id))
    }

    /// presetID turns a preset name into an ID the backend accepts, steering clear of built-in IDs.
    static func presetID(for name: String, taken: [String]) -> String {
        var id = ""
        for ch in name.lowercased() {
            if ch.isASCII && (ch.isLetter || ch.isNumber) {
                id.append(ch)
            } else if !id.isEmpty && id.last != "-" {
                id.append("-")
            }
        }
        while id.hasSuffix("-") { id.removeLast() }
        id = String(id.prefix(56))
        if id.isEmpty { id = "preset" }
        if taken.contains(id) || id == "recent" { id += "-custom" }
        return id
    }

    /// cancel stops a queued or running item.
    public func cancel(_ id: UUID) {
        guard let item = items.first(where: { $0.id == id }), let job = item.jobID else { return }
        send(Request(id: "cancel:" + id.uuidString, op: "cancel", job: job))
    }

    /// retry puts a failed or canceled item back to pending.
    public func retry(_ id: UUID) {
        guard let i = items.firstIndex(where: { $0.id == id }), items[i].phase == .failed || items[i].phase == .canceled,
              items[i].probeError == nil else { return }
        let old = items[i]
        items[i] = Item(id: UUID(), url: old.url, info: old.info, settings: old.settings)
        if selection == id { selection = items[i].id }
    }

    /// remove drops an item that is not queued or running.
    public func remove(_ id: UUID) {
        items.removeAll { $0.id == id && !($0.phase == .queued || $0.phase == .running) }
        clearStaleSelection()
        pruneThumbnails()
    }

    /// canRemoveSelection is true when a file is selected and not queued or running.
    public var canRemoveSelection: Bool {
        guard let id = selection, let item = items.first(where: { $0.id == id }) else { return false }
        return !(item.phase == .queued || item.phase == .running)
    }

    /// removeSelection drops the selected file when it is not queued or running.
    public func removeSelection() {
        if let id = selection { remove(id) }
    }

    /// row is the display model of the file with id, or nil when it is gone.
    public func row(_ id: UUID) -> RowModel? {
        guard let item = items.first(where: { $0.id == id }) else { return nil }
        return RowModel(item: item, windowSettings: settings, catalog: catalog, targetSize: targetSize,
                        hardware: hardware)
    }

    /// clearFinished drops finished items here and in the converter.
    public func clearFinished() {
        items.removeAll { $0.phase.isFinished }
        clearStaleSelection()
        pruneThumbnails()
        send(Request(id: "clear", op: "clear"))
    }

    /// requestThumbnail makes the row frame for url once, off the main actor.
    private func requestThumbnail(_ url: URL) {
        guard thumbnailTasks[url] == nil else { return }
        let make = thumbnail
        thumbnailTasks[url] = Task { [weak self] in
            let image = await make(url)
            self?.storeThumbnail(image, for: url)
        }
    }

    /// storeThumbnail records a finished frame for a source still in the list. It is a plain
    /// method so the dictionary write runs outside the async closure.
    private func storeThumbnail(_ image: CGImage?, for url: URL) {
        guard let image, thumbnailTasks[url] != nil else { return }
        thumbnails[url] = image
    }

    /// pruneThumbnails drops frames for sources no longer in the list.
    private func pruneThumbnails() {
        let urls = Set(items.map(\.url))
        for url in thumbnailTasks.keys where !urls.contains(url) {
            thumbnailTasks[url]?.cancel()
            thumbnailTasks[url] = nil
            thumbnails[url] = nil
        }
    }

    /// clearStaleSelection drops a selection whose file is gone.
    private func clearStaleSelection() {
        if let id = selection, !items.contains(where: { $0.id == id }) { selection = nil }
    }

    /// backendExited records that the converter stopped and fails everything in flight.
    public func backendExited(_ message: String) {
        fault = message
        for i in items.indices where items[i].phase == .queued || items[i].phase == .running {
            items[i].addError = "The converter stopped."
            items[i].status = nil
        }
    }

    /// handle applies one backend message.
    public func handle(_ m: Message) {
        let (kind, itemID) = Converter.split(m.reply)
        switch m.type {
        case "ready":
            fault = nil
            version = m.version
            send(Request(id: "presets", op: "presets"))
            send(Request(id: "capabilities", op: "capabilities"))
            send(Request(id: "catalog", op: "catalog"))
        case "presets", "preset.save", "preset.delete":
            applyPresets(m)
        case "catalog":
            catalog = m.catalog
        case "capabilities":
            encoders = m.encoders ?? []
            if !hasHardware { hardware = false }
        case "probe":
            update(itemID) { $0.info = m.info }
            if let id = itemID, let item = items.first(where: { $0.id == id }), m.info?.video != nil {
                requestThumbnail(item.url)
            }
        case "add":
            guard let status = m.status else { return }
            update(itemID) { $0.jobID = status.id }
            apply(early.removeValue(forKey: status.id) ?? status)
        case "job":
            guard let status = m.status else { return }
            if items.contains(where: { $0.jobID == status.id }) { apply(status) } else { early[status.id] = status }
        case "error":
            let text = m.error ?? "Unknown error."
            switch kind {
            case "probe": update(itemID) { $0.probeError = text }
            case "add": update(itemID) { $0.addError = text }
            case "cancel": break
            case "preset.save", "preset.delete":
                savingID = nil
                presetError = text
            default: fault = text
            }
        default:
            break
        }
    }

    /// applyPresets takes a presets list. A finished save selects the saved preset; a selection
    /// that is gone falls back to the first preset; edited settings are left alone otherwise.
    private func applyPresets(_ m: Message) {
        presets = m.presets ?? []
        recent = m.recent ?? recent
        presetError = m.warning
        if m.type == "preset.save", let id = savingID {
            savingID = nil
            presetID = id
        } else if !presetID.isEmpty, preset(id: presetID) == nil, let first = presets.first {
            presetID = first.id
        } else if settings == nil, let p = preset(id: presetID) {
            settings = p
        }
    }

    /// apply records a job snapshot on its item.
    private func apply(_ status: JobStatus) {
        guard let i = items.firstIndex(where: { $0.jobID == status.id }) else { return }
        if let old = items[i].status, old.state.isFinished, !status.state.isFinished { return }
        items[i].status = status
        if status.state == .done {
            items[i].outputSize = fileSize(URL(fileURLWithPath: status.job.output))
        }
    }

    /// update mutates the item with id.
    private func update(_ id: UUID?, _ change: (inout Item) -> Void) {
        guard let id, let i = items.firstIndex(where: { $0.id == id }) else { return }
        change(&items[i])
    }

    /// split parses a reply token of the form "kind:uuid".
    static func split(_ reply: String?) -> (String, UUID?) {
        guard let reply, let colon = reply.firstIndex(of: ":") else { return (reply ?? "", nil) }
        return (String(reply[..<colon]), UUID(uuidString: String(reply[reply.index(after: colon)...])))
    }
}

/// InspectorScope is what the inspector edits.
public enum InspectorScope: Equatable, Sendable {
    /// window edits the settings for every file without its own.
    case window
    /// item is the selected pending file, named name, which follows the window settings when
    /// usesWindow is true.
    case item(UUID, name: String, usesWindow: Bool)
}
