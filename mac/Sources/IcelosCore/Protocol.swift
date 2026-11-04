import Foundation

/// Preset mirrors one built-in or custom conversion target from the backend.
public struct Preset: Codable, Hashable, Identifiable, Sendable {
    /// Video holds the video half of a preset.
    public struct Video: Codable, Hashable, Sendable {
        /// codec is a catalog codec ID, "copy", or empty for audio-only output.
        public var codec: String
        /// maxWidth caps the long side in pixels.
        public var maxWidth: Int?
        /// maxHeight caps the short side in pixels.
        public var maxHeight: Int?
        /// width is the exact output width in pixels.
        public var width: Int?
        /// height is the exact output height in pixels.
        public var height: Int?
        /// fps is the output frame rate.
        public var fps: Double?
        /// rateControl is "quality", "cbr", "vbr", or "twopass".
        public var rateControl: String?
        /// bitrate is in bits per second.
        public var bitrate: Int?
        /// profile is the codec profile ID.
        public var profile: String?
        /// level is the codec level.
        public var level: String?
        /// tag is the container codec tag.
        public var tag: String?
        /// quality is the constant-quality step.
        public var quality: Int?
    }

    /// Audio holds the audio half of a preset.
    public struct Audio: Codable, Hashable, Sendable {
        /// codec is a catalog codec ID, "copy", or empty for silent output.
        public var codec: String
        /// bitrate is in bits per second.
        public var bitrate: Int?
        /// channels caps the channel count.
        public var channels: Int?
        /// sampleRate is in hertz.
        public var sampleRate: Int?
        /// bitDepth is the sample depth in bits.
        public var bitDepth: Int?
    }

    /// id is the stable preset identifier.
    public var id: String
    /// name is the short label.
    public var name: String
    /// description says who the preset is for.
    public var description: String
    /// container is the output container ID.
    public var container: String?
    /// ext is the output extension without the dot.
    public var ext: String
    /// video is the video settings.
    public var video: Video
    /// audio is the audio settings.
    public var audio: Audio
    /// targetBytes is the desired output size when set.
    public var targetBytes: Int64?
    /// custom is true for presets the user saved.
    public var custom: Bool?

    /// needsSize is true for the preset that asks the user for a size.
    public var needsSize: Bool { id == "target-size" }

    /// isAudioOnly is true when the preset drops video.
    public var isAudioOnly: Bool { video.codec.isEmpty }

    /// isCustom is true for presets the user saved.
    public var isCustom: Bool { custom == true }
}

/// JobProgress mirrors one backend progress report.
public struct JobProgress: Codable, Hashable, Sendable {
    /// percent is overall completion from 0 to 1.
    public var percent: Double
    /// pass is the current pass, starting at 1.
    public var pass: Int
    /// passes is the total pass count.
    public var passes: Int
    /// elapsedNs is the media time encoded so far in this pass.
    public var elapsedNs: Int64
    /// speed is the encode speed as a multiple of real time.
    public var speed: Double
    /// fps is the encoding frame rate.
    public var fps: Double
    /// etaNs is the estimated time left, or zero when unknown.
    public var etaNs: Int64
    /// encoder is the ffmpeg video encoder in use.
    public var encoder: String?
    /// hardware is true when the media engine is encoding.
    public var hardware: Bool
}

/// JobState mirrors the backend job states.
public enum JobState: String, Codable, Sendable {
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

    /// isFinished is true for final states.
    public var isFinished: Bool { self == .done || self == .failed || self == .canceled }
}

/// Job mirrors the backend conversion request.
public struct Job: Codable, Hashable, Sendable {
    /// input is the source path.
    public var input: String
    /// output is the destination path.
    public var output: String
    /// preset is the conversion target.
    public var preset: Preset
    /// hardware prefers VideoToolbox.
    public var hardware: Bool
    /// passthrough copies the streams that already match the target.
    public var passthrough: Bool?
    /// streams chooses the tracks and extras the output carries.
    public var streams: Streams?
    /// range limits the conversion to part of the source.
    public var range: TimeRange?
    /// splitChapters asks for one output per chapter.
    public var splitChapters: Bool?

    /// Creates a job.
    public init(input: String, output: String, preset: Preset, hardware: Bool, passthrough: Bool? = nil,
                streams: Streams? = nil, range: TimeRange? = nil, splitChapters: Bool? = nil) {
        self.input = input
        self.output = output
        self.preset = preset
        self.hardware = hardware
        self.passthrough = passthrough
        self.streams = streams
        self.range = range
        self.splitChapters = splitChapters
    }
}

/// TimeRange mirrors the part of a source a job converts. Nil bounds mean the start or the end.
public struct TimeRange: Codable, Hashable, Sendable {
    /// startNs is where the conversion starts.
    public var startNs: Int64?
    /// endNs is where the conversion stops.
    public var endNs: Int64?
}

/// SubtitleMode says what a job does with subtitles.
public enum SubtitleMode: String, Codable, CaseIterable, Sendable {
    /// none leaves subtitles out.
    case none
    /// keep carries every subtitle track the output format can hold.
    case keep
    /// pick carries one subtitle track.
    case pick
    /// burn draws one subtitle track into the picture.
    case burn
}

/// Subtitles mirrors what a job does with subtitles.
public struct Subtitles: Codable, Hashable, Sendable {
    /// mode is what happens to subtitles, nil for none.
    public var mode: SubtitleMode?
    /// track is the position among the source's subtitle streams that pick and burn use.
    public var track: Int?
    /// external is a text subtitle file that keep adds and pick and burn use instead of track.
    public var external: String?

    /// Creates subtitle choices.
    public init(mode: SubtitleMode? = nil, track: Int? = nil, external: String? = nil) {
        self.mode = mode
        self.track = track
        self.external = external
    }
}

/// Streams mirrors the tracks and extras a job's output carries. Nil fields keep the backend's
/// defaults: the first audio track, no subtitles, the source's chapters and metadata, and ffmpeg's
/// own thread count.
public struct Streams: Codable, Hashable, Sendable {
    /// allTracks as the only audio track keeps every audio track.
    public static let allTracks = -1
    /// maxExternalAudio is the most audio files one job may add.
    public static let maxExternalAudio = 6
    /// subtitleExtensions are the subtitle file types the backend takes.
    public static let subtitleExtensions = ["srt", "ass", "ssa", "vtt"]

    /// audioTracks lists positions among the source's audio streams, from 0.
    public var audioTracks: [Int]?
    /// externalAudio lists audio files added as extra tracks.
    public var externalAudio: [String]?
    /// subtitles chooses the subtitle handling.
    public var subtitles: Subtitles?
    /// dropChapters leaves the source's chapters out.
    public var dropChapters: Bool?
    /// dropMetadata leaves the source's tags out.
    public var dropMetadata: Bool?
    /// threads caps the encoder threads.
    public var threads: Int?

    /// Creates stream choices.
    public init(audioTracks: [Int]? = nil, externalAudio: [String]? = nil, subtitles: Subtitles? = nil,
                dropChapters: Bool? = nil, dropMetadata: Bool? = nil, threads: Int? = nil) {
        self.audioTracks = audioTracks
        self.externalAudio = externalAudio
        self.subtitles = subtitles
        self.dropChapters = dropChapters
        self.dropMetadata = dropMetadata
        self.threads = threads
    }

    /// normalized returns the same choices with every default spelled as nil, so equal choices
    /// compare equal and an all-default value sends nothing.
    public var normalized: Streams {
        var s = self
        if s.audioTracks == [] || s.audioTracks == [0] { s.audioTracks = nil }
        if s.externalAudio?.isEmpty == true { s.externalAudio = nil }
        if var sub = s.subtitles {
            if sub.mode == SubtitleMode.none { sub.mode = nil }
            if sub.track == 0 { sub.track = nil }
            if sub.external?.isEmpty == true { sub.external = nil }
            s.subtitles = sub == Subtitles() ? nil : sub
        }
        if s.dropChapters == false { s.dropChapters = nil }
        if s.dropMetadata == false { s.dropMetadata = nil }
        if let t = s.threads, t <= 0 { s.threads = nil }
        return s
    }

    /// isDefault is true when every choice is the backend default.
    public var isDefault: Bool { normalized == Streams() }

    /// tracks returns the audio positions kept from a source with count audio streams.
    public func tracks(count: Int) -> [Int] {
        guard count > 0 else { return [] }
        let list = audioTracks ?? []
        if list.isEmpty { return [0] }
        if list == [Streams.allTracks] { return Array(0..<count) }
        return list.filter { $0 >= 0 && $0 < count }.sorted()
    }

    /// settingTrack returns the choices with the audio track at position turned on or off, from a
    /// source with count audio streams. The last kept track cannot be turned off.
    public func settingTrack(_ position: Int, on: Bool, count: Int) -> Streams {
        var kept = Set(tracks(count: count))
        if on { kept.insert(position) } else { kept.remove(position) }
        guard !kept.isEmpty else { return self }
        var s = self
        s.audioTracks = kept.count == count && count > 1 ? [Streams.allTracks] : kept.sorted()
        return s.normalized
    }

    /// subtitleMode is the subtitle mode, none when unset.
    public var subtitleMode: SubtitleMode { subtitles?.mode ?? SubtitleMode.none }
}

/// Verdict mirrors whether one kind of stream passes through untouched.
public struct Verdict: Codable, Hashable, Sendable {
    /// copy is true when the stream passes through.
    public var copy: Bool
    /// reason says why the stream is encoded.
    public var reason: String?

    /// Creates a verdict.
    public init(copy: Bool, reason: String? = nil) {
        self.copy = copy
        self.reason = reason
    }
}

/// Copy mirrors the passthrough verdict for each kind of output stream. A nil verdict means the
/// output has no such stream.
public struct Copy: Codable, Hashable, Sendable {
    /// video is the video verdict.
    public var video: Verdict?
    /// audio is the verdict for every audio track together.
    public var audio: Verdict?

    /// Creates verdicts.
    public init(video: Verdict? = nil, audio: Verdict? = nil) {
        self.video = video
        self.audio = audio
    }

    /// lossless is true when the output has a stream and every one of them copies.
    public var lossless: Bool {
        let present = [video, audio].compactMap { $0 }
        return !present.isEmpty && present.allSatisfy(\.copy)
    }
}

/// Plan mirrors the backend's answer to a plan request: what an add would run.
public struct Plan: Codable, Hashable, Sendable {
    /// copy says which streams could pass through.
    public var copy: Copy
    /// copied says which streams the conversion copies.
    public var copied: Copy
    /// lossless is true when the conversion copies every output stream.
    public var lossless: Bool
    /// output is the path the conversion would write.
    public var output: String
    /// passes holds the ffmpeg arguments of each pass.
    public var passes: [[String]]
    /// encoder is the video encoder, nil when video is copied or absent.
    public var encoder: String?
    /// hardware is true when the encoder runs on the media engine.
    public var hardware: Bool
    /// filters lists filters that some ffmpeg builds lack.
    public var filters: [String]?
    /// estimate is the expected output size, a range in quality mode and one size otherwise, nil
    /// when the backend has nothing to go on.
    public var estimate: Estimate.Range?

    /// Creates a plan.
    public init(copy: Copy, copied: Copy, lossless: Bool, output: String = "", passes: [[String]] = [],
                encoder: String? = nil, hardware: Bool = false, filters: [String]? = nil,
                estimate: Estimate.Range? = nil) {
        self.copy = copy
        self.copied = copied
        self.lossless = lossless
        self.output = output
        self.passes = passes
        self.encoder = encoder
        self.hardware = hardware
        self.filters = filters
        self.estimate = estimate
    }
}

/// JobStatus mirrors a backend job snapshot.
public struct JobStatus: Codable, Hashable, Sendable {
    /// id is the backend job identifier.
    public var id: String
    /// job is the request with its output resolved.
    public var job: Job
    /// state is where the job is.
    public var state: JobState
    /// progress is the latest report.
    public var progress: JobProgress
    /// error is the failure message.
    public var error: String?
}

/// MediaStream mirrors one probed media stream.
public struct MediaStream: Codable, Hashable, Sendable {
    /// index is the stream position.
    public var index: Int
    /// kind is "video", "audio", or "subtitle".
    public var kind: String
    /// codec is the codec short name.
    public var codec: String?
    /// width is the coded frame width.
    public var width: Int?
    /// height is the coded frame height.
    public var height: Int?
    /// channels is the audio channel count.
    public var channels: Int?
    /// disposition carries the stream role flags.
    public var disposition: Disposition?
    /// sideData carries per-stream extras such as the display rotation.
    public var sideData: [SideData]?
    /// tags carries the stream's language and title.
    public var tags: Tags?

    /// Tags mirrors the stream tags the backend reads.
    public struct Tags: Codable, Hashable, Sendable {
        /// language is the ISO 639 language code.
        public var language: String?
        /// title is the track name.
        public var title: String?
    }

    /// Disposition mirrors the ffprobe role flags.
    public struct Disposition: Codable, Hashable, Sendable {
        /// attachedPic is 1 when the stream is embedded cover art.
        public var attachedPic: Int?

        /// CodingKeys maps ffprobe field names.
        enum CodingKeys: String, CodingKey {
            case attachedPic = "attached_pic"
        }
    }

    /// SideData mirrors one ffprobe side data entry.
    public struct SideData: Codable, Hashable, Sendable {
        /// rotation is the display rotation in degrees.
        public var rotation: Int?
    }

    /// isCover is true for embedded cover art rather than motion video.
    public var isCover: Bool { disposition?.attachedPic == 1 }

    /// displaySize is the frame size as the viewer sees it, swapped for a quarter-turn rotation,
    /// as the backend reads it.
    public var displaySize: (width: Int, height: Int)? {
        guard let w = width, let h = height, w > 0, h > 0 else { return nil }
        let quarter = (sideData ?? []).contains {
            let r = (($0.rotation ?? 0) % 360 + 360) % 360
            return r == 90 || r == 270
        }
        return quarter ? (h, w) : (w, h)
    }

    /// CodingKeys maps ffprobe field names.
    enum CodingKeys: String, CodingKey {
        case index, width, height, channels, disposition, tags
        case kind = "codec_type"
        case codec = "codec_name"
        case sideData = "side_data_list"
    }
}

/// MediaInfo mirrors the backend probe result.
public struct MediaInfo: Codable, Hashable, Sendable {
    /// format is the container name list.
    public var format: String
    /// durationNs is the playing time.
    public var durationNs: Int64
    /// size is the file size in bytes.
    public var size: Int64
    /// streams lists every stream.
    public var streams: [MediaStream]?
    /// chapters lists the source's chapter marks.
    public var chapters: [Chapter]?

    /// Chapter mirrors one chapter mark.
    public struct Chapter: Codable, Hashable, Sendable {
        /// startNs is where the chapter starts.
        public var startNs: Int64
        /// endNs is where the chapter ends.
        public var endNs: Int64
        /// title is the chapter name.
        public var title: String?
    }

    /// audioStreams lists the audio streams in source order, whose positions Streams uses.
    public var audioStreams: [MediaStream] { streams?.filter { $0.kind == "audio" } ?? [] }

    /// subtitleStreams lists the subtitle streams in source order, whose positions Subtitles uses.
    public var subtitleStreams: [MediaStream] { streams?.filter { $0.kind == "subtitle" } ?? [] }

    /// video is the first video stream that is not cover art.
    public var video: MediaStream? { streams?.first { $0.kind == "video" && !$0.isCover } }

    /// audio is the first audio stream.
    public var audio: MediaStream? { streams?.first { $0.kind == "audio" } }
}

/// Request is one line sent to the backend.
public struct Request: Codable, Sendable, Equatable {
    /// id is echoed in the reply.
    public var id: String?
    /// op is the operation name.
    public var op: String
    /// path is the file to probe.
    public var path: String?
    /// input is the file to convert.
    public var input: String?
    /// preset is the preset ID.
    public var preset: String?
    /// presetBody is a whole preset, sent instead of preset for edited settings and saves.
    public var presetBody: Preset?
    /// size is the target size, e.g. "25MB".
    public var size: String?
    /// hardware prefers VideoToolbox.
    public var hardware: Bool?
    /// outDir is the output folder.
    public var outDir: String?
    /// streams chooses the tracks and extras the output carries.
    public var streams: Streams?
    /// splitChapters asks for one output per chapter.
    public var splitChapters: Bool?
    /// passthrough copies the streams that already match the target.
    public var passthrough: Bool?
    /// job is the job ID to cancel.
    public var job: String?

    /// Creates a request.
    public init(id: String? = nil, op: String, path: String? = nil, input: String? = nil,
                preset: String? = nil, presetBody: Preset? = nil, size: String? = nil,
                hardware: Bool? = nil, outDir: String? = nil, streams: Streams? = nil,
                splitChapters: Bool? = nil, passthrough: Bool? = nil, job: String? = nil) {
        self.id = id
        self.op = op
        self.path = path
        self.input = input
        self.preset = preset
        self.presetBody = presetBody
        self.size = size
        self.hardware = hardware
        self.outDir = outDir
        self.streams = streams
        self.splitChapters = splitChapters
        self.passthrough = passthrough
        self.job = job
    }
}

/// Message is one line received from the backend.
public struct Message: Codable, Sendable {
    /// type is the message kind or the op it answers.
    public var type: String
    /// reply is the ID of the request this answers.
    public var reply: String?
    /// error is the failure message.
    public var error: String?
    /// version is the backend version.
    public var version: String?
    /// encoders lists usable ffmpeg encoders.
    public var encoders: [String]?
    /// presets lists the built-in presets, then the custom ones.
    public var presets: [Preset]?
    /// recent lists recently used preset IDs, newest first.
    public var recent: [String]?
    /// warning reports a problem that did not stop the request.
    public var warning: String?
    /// catalog is the format catalog.
    public var catalog: Catalog?
    /// info is a probe result.
    public var info: MediaInfo?
    /// plan is the answer to a plan request.
    public var plan: Plan?
    /// status is one job snapshot.
    public var status: JobStatus?
    /// jobs lists every job.
    public var jobs: [JobStatus]?
}
