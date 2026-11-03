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
        case index, width, height, channels, disposition
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

    /// video is the first video stream that is not cover art.
    public var video: MediaStream? { streams?.first { $0.kind == "video" && !$0.isCover } }

    /// bitrate is the overall bit rate in bits per second, or nil when the size or duration is
    /// unknown.
    public var bitrate: Double? {
        guard size > 0, durationNs > 0 else { return nil }
        return Double(size) * 8 / (Double(durationNs) / 1e9)
    }

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
    /// job is the job ID to cancel.
    public var job: String?

    /// Creates a request.
    public init(id: String? = nil, op: String, path: String? = nil, input: String? = nil,
                preset: String? = nil, presetBody: Preset? = nil, size: String? = nil,
                hardware: Bool? = nil, outDir: String? = nil, job: String? = nil) {
        self.id = id
        self.op = op
        self.path = path
        self.input = input
        self.preset = preset
        self.presetBody = presetBody
        self.size = size
        self.hardware = hardware
        self.outDir = outDir
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
    /// status is one job snapshot.
    public var status: JobStatus?
    /// jobs lists every job.
    public var jobs: [JobStatus]?
}
