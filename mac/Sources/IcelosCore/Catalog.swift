import Foundation

/// Encoder mirrors one ffmpeg encoder for a codec.
public struct Encoder: Codable, Hashable, Sendable {
    /// name is the ffmpeg encoder name.
    public var name: String
    /// hardware is true for media-engine encoders.
    public var hardware: Bool?
    /// maxSampleRate caps the sample rate the encoder accepts.
    public var maxSampleRate: Int?
}

/// Profile mirrors one codec profile.
public struct Profile: Codable, Hashable, Sendable {
    /// id is the profile identifier.
    public var id: String
    /// name is the label.
    public var name: String
    /// pixFmt is the pixel format the profile requires.
    public var pixFmt: String?
}

/// Mode mirrors one fixed video mode.
public struct Mode: Codable, Hashable, Sendable {
    /// name is the label.
    public var name: String
    /// profile is the profile ID the mode belongs to.
    public var profile: String?
    /// width is the frame width in pixels.
    public var width: Int
    /// height is the frame height in pixels.
    public var height: Int
    /// fps is the frame rate.
    public var fps: Double?
    /// bitrates lists the allowed bitrates.
    public var bitrates: [Int]?
    /// pixFmt is the pixel format.
    public var pixFmt: String?
}

/// BitrateCap mirrors one audio bitrate ceiling.
public struct BitrateCap: Codable, Hashable, Sendable {
    /// minRate is the lowest sample rate the cap applies to.
    public var minRate: Int?
    /// total is the ceiling for the whole stream.
    public var total: Int?
    /// perChannel is the ceiling per channel.
    public var perChannel: Int?
    /// perSample is the ceiling per sample.
    public var perSample: Int?
}

/// Flags mirrors the container muxing flags.
public struct Flags: Codable, Hashable, Sendable {
    /// faststart moves the index to the front.
    public var faststart: Bool?
    /// hvc1Tag tags HEVC as hvc1.
    public var hvc1Tag: Bool?
    /// m2ts selects the transport stream layout.
    public var m2ts: Bool?
    /// shortest ends output with the shortest stream.
    public var shortest: Bool?
}

/// Container mirrors one output container.
public struct Container: Codable, Hashable, Identifiable, Sendable {
    /// id is the container identifier.
    public var id: String
    /// name is the label.
    public var name: String
    /// ext is the file extension without the dot.
    public var ext: String
    /// muxer is the ffmpeg muxer name.
    public var muxer: String
    /// kind is "video" or "audio".
    public var kind: String
    /// video lists allowed video codec IDs.
    public var video: [String]?
    /// audio lists allowed audio codec IDs.
    public var audio: [String]?
    /// subtitles lists allowed subtitle codec IDs.
    public var subtitles: [String]?
    /// sampleRates lists the allowed sample rates.
    public var sampleRates: [Int]?
    /// channels caps the channel count.
    public var channels: Int?
    /// modes lists fixed video modes.
    public var modes: [Mode]?
    /// flags holds the muxing flags.
    public var flags: Flags
}

/// Codec mirrors one catalog codec and whether it can be encoded here.
public struct Codec: Codable, Hashable, Identifiable, Sendable {
    /// id is the codec identifier.
    public var id: String
    /// name is the label.
    public var name: String
    /// kind is "video", "audio", or "subtitle".
    public var kind: String
    /// encoders lists the ffmpeg encoders.
    public var encoders: [Encoder]
    /// rateControls lists the supported rate controls.
    public var rateControls: [String]?
    /// profiles lists the codec profiles.
    public var profiles: [Profile]?
    /// levels lists the codec levels.
    public var levels: [String]?
    /// tags lists the container tags.
    public var tags: [String]?
    /// pixFmt is the pixel format.
    public var pixFmt: String?
    /// modes lists fixed video modes.
    public var modes: [Mode]?
    /// frameRates lists the allowed frame rates.
    public var frameRates: [Double]?
    /// lossless is true for lossless codecs.
    public var lossless: Bool?
    /// sampleRates lists the allowed sample rates.
    public var sampleRates: [Int]?
    /// maxChannels caps the channel count.
    public var maxChannels: Int?
    /// bitrates lists the allowed bitrates.
    public var bitrates: [Int]?
    /// bitrateCaps lists the bitrate ceilings.
    public var bitrateCaps: [BitrateCap]?
    /// bitDepths lists the allowed bit depths.
    public var bitDepths: [Int]?
    /// available is true when this ffmpeg build can encode the codec.
    public var available: Bool
}

/// Resolution mirrors one resolution choice.
public struct Resolution: Codable, Hashable, Identifiable, Sendable {
    /// id is the resolution identifier.
    public var id: String
    /// name is the label.
    public var name: String
    /// width is the frame width, or zero for the source.
    public var width: Int
    /// height is the frame height, or zero for the source.
    public var height: Int
}

/// FrameRate mirrors one frame rate choice.
public struct FrameRate: Codable, Hashable, Sendable {
    /// name is the label.
    public var name: String
    /// fps is the rate, or zero for the source.
    public var fps: Double
}

/// ChannelLayout mirrors one channel count choice.
public struct ChannelLayout: Codable, Hashable, Sendable {
    /// count is the channel count.
    public var count: Int
    /// name is the label.
    public var name: String
}

/// Domains mirrors the value lists the settings dialog offers.
public struct Domains: Codable, Hashable, Sendable {
    /// resolutions lists the resolution choices.
    public var resolutions: [Resolution]
    /// minDimension is the smallest allowed width or height.
    public var minDimension: Int
    /// maxDimension is the largest allowed width or height.
    public var maxDimension: Int
    /// frameRates lists the frame rate choices.
    public var frameRates: [FrameRate]
    /// videoBitrates lists the video bitrate choices.
    public var videoBitrates: [Int]
    /// audioBitrates lists the audio bitrate choices.
    public var audioBitrates: [Int]
    /// sampleRates lists the sample rate choices.
    public var sampleRates: [Int]
    /// channels lists the channel layout choices.
    public var channels: [ChannelLayout]
}

/// Catalog mirrors the backend format catalog.
public struct Catalog: Codable, Hashable, Sendable {
    /// containers lists every output container.
    public var containers: [Container]
    /// codecs lists every codec with its availability.
    public var codecs: [Codec]
    /// domains holds the settings value lists.
    public var domains: Domains

    /// videoContainers lists the containers whose main stream is video.
    public var videoContainers: [Container] { containers.filter { $0.kind == "video" } }

    /// audioContainers lists the audio-only containers.
    public var audioContainers: [Container] { containers.filter { $0.kind == "audio" } }

    /// codecs returns the codecs a container allows, in catalog order.
    public func codecs(for container: Container) -> [Codec] {
        let allowed = Set((container.video ?? []) + (container.audio ?? []) + (container.subtitles ?? []))
        return codecs.filter { allowed.contains($0.id) }
    }

    /// isAvailable reports whether the codec can be encoded by this ffmpeg build.
    public func isAvailable(codecID: String) -> Bool {
        codecs.first { $0.id == codecID }?.available ?? false
    }
}
