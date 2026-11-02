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
    /// presets are the built-in presets in display order.
    public private(set) var presets: [Preset] = []
    /// items are the files in the window in the order they were added.
    public private(set) var items: [Item] = []
    /// encoders lists the usable ffmpeg encoders.
    public private(set) var encoders: [String] = []
    /// version is the converter version.
    public private(set) var version: String?
    /// fault is a problem with the converter itself.
    public private(set) var fault: String?
    /// presetID is the selected preset.
    public var presetID = "apple"
    /// hardware prefers the media engine.
    public var hardware = true
    /// targetSize is the size for the target-size preset.
    public var targetSize = "25MB"
    /// outputFolder is where outputs go, or nil for next to each source.
    public var outputFolder: URL?

    /// send delivers a request to the backend.
    @ObservationIgnored private let send: (Request) -> Void
    /// early holds job events that arrived before their add reply.
    @ObservationIgnored private var early: [String: JobStatus] = [:]
    /// fileSize reads a file size, injectable for tests.
    @ObservationIgnored private let fileSize: (URL) -> Int64?

    /// Creates a converter that sends requests through send.
    public init(send: @escaping (Request) -> Void,
                fileSize: @escaping (URL) -> Int64? = Converter.diskSize) {
        self.send = send
        self.fileSize = fileSize
    }

    /// diskSize returns the size of the file at url.
    public nonisolated static func diskSize(_ url: URL) -> Int64? {
        (try? FileManager.default.attributesOfItem(atPath: url.path)[.size] as? NSNumber)?.int64Value
    }

    /// selectedPreset is the preset for presetID.
    public var selectedPreset: Preset? { presets.first { $0.id == presetID } }

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

    /// convertAll submits every pending, readable item with the current settings.
    public func convertAll() {
        guard let preset = selectedPreset else { return }
        for i in items.indices where items[i].phase == .pending && items[i].probeError == nil {
            items[i].submitted = true
            send(Request(id: "add:" + items[i].id.uuidString, op: "add", input: items[i].url.path,
                         preset: preset.id, size: preset.needsSize ? targetSize : nil,
                         hardware: hardware, outDir: outputFolder?.path))
        }
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
        items[i] = Item(id: UUID(), url: old.url, info: old.info)
    }

    /// remove drops an item that is not queued or running.
    public func remove(_ id: UUID) {
        items.removeAll { $0.id == id && !($0.phase == .queued || $0.phase == .running) }
    }

    /// clearFinished drops finished items here and in the converter.
    public func clearFinished() {
        items.removeAll { $0.phase.isFinished }
        send(Request(id: "clear", op: "clear"))
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
        case "presets":
            presets = m.presets ?? []
            if !presets.contains(where: { $0.id == presetID }), let first = presets.first { presetID = first.id }
        case "capabilities":
            encoders = m.encoders ?? []
            if !hasHardware { hardware = false }
        case "probe":
            update(itemID) { $0.info = m.info }
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
            default: fault = text
            }
        default:
            break
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
