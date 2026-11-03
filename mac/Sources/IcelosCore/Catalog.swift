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

/// Quality levels mirror the backend's codec-neutral quality steps.
public enum QualityLevel {
    /// high keeps nearly all visible detail.
    public static let high = 1
    /// balanced trades a little detail for a much smaller file.
    public static let balanced = 2
    /// small favors file size over detail.
    public static let small = 3
}

/// RateControl names the backend rate controls.
public enum RateControl {
    /// quality holds a constant quality level.
    public static let quality = "quality"
    /// cbr holds a constant bitrate.
    public static let cbr = "cbr"
    /// vbr aims for an average bitrate.
    public static let vbr = "vbr"
    /// twopass encodes twice to land on a bitrate or a size.
    public static let twopass = "twopass"
}

/// copyCodec is the codec ID that passes a source stream through untouched.
public let copyCodec = "copy"

extension Catalog {
    /// defaultVideoBitrate is the bitrate a bitrate rate control starts from.
    static let defaultVideoBitrate = 8_000_000
    /// defaultAudioBitrate is the bitrate a lossy audio codec starts from.
    static let defaultAudioBitrate = 192_000

    /// container returns the container with id.
    public func container(id: String?) -> Container? {
        containers.first { $0.id == id }
    }

    /// codec returns the codec with id.
    public func codec(id: String?) -> Codec? {
        codecs.first { $0.id == id }
    }

    /// videoCodecs lists the video codecs a container carries, in the container's order.
    public func videoCodecs(in container: Container) -> [Codec] {
        (container.video ?? []).compactMap { codec(id: $0) }
    }

    /// audioCodecs lists the audio codecs a container carries, in the container's order.
    public func audioCodecs(in container: Container) -> [Codec] {
        (container.audio ?? []).compactMap { codec(id: $0) }
    }

    /// isUsable reports whether the container can be written with at least one available codec.
    public func isUsable(_ container: Container) -> Bool {
        let main = container.kind == "video" ? videoCodecs(in: container) : audioCodecs(in: container)
        return main.contains { $0.available }
    }

    /// modes lists the fixed frame shapes for a codec and profile in a container. The container's
    /// modes win when it has any, as in the backend.
    public func modes(for codec: Codec, profile: String?, in container: Container) -> [Mode] {
        if let modes = container.modes, !modes.isEmpty { return modes }
        return (codec.modes ?? []).filter { $0.profile == nil || $0.profile == profile }
    }

    /// isFixed reports whether the codec and container take only fixed frame shapes, so a fit box
    /// is refused.
    public func isFixed(_ codec: Codec, profile: String?, in container: Container) -> Bool {
        !modes(for: codec, profile: profile, in: container).isEmpty
    }

    /// frameRates lists the frame rate choices the codec accepts.
    public func frameRates(for codec: Codec) -> [FrameRate] {
        guard let allowed = codec.frameRates, !allowed.isEmpty else { return domains.frameRates }
        return domains.frameRates.filter { r in r.fps == 0 || allowed.contains { abs($0 - r.fps) < 0.01 } }
    }

    /// sampleRates lists the sample rates the codec, the container, and at least one encoder take.
    public func sampleRates(for codec: Codec, in container: Container) -> [Int] {
        domains.sampleRates.filter { rate in
            (codec.sampleRates.map { $0.isEmpty || $0.contains(rate) } ?? true)
                && (container.sampleRates.map { $0.isEmpty || $0.contains(rate) } ?? true)
                && codec.encoders.contains { rate <= ($0.maxSampleRate ?? Int.max) }
        }
    }

    /// channelLayouts lists the channel layouts the codec and container carry.
    public func channelLayouts(for codec: Codec, in container: Container) -> [ChannelLayout] {
        domains.channels.filter { l in
            (codec.maxChannels.map { l.count <= $0 } ?? true)
                && (container.channels.map { l.count == $0 } ?? true)
        }
    }

    /// maxAudioBitrate returns the highest bitrate the codec takes at a sample rate and channel
    /// count, or zero when it has no cap. Zero rates and counts take the most the codec allows,
    /// as the backend does.
    public func maxAudioBitrate(for codec: Codec, sampleRate: Int?, channels: Int?) -> Int {
        var rate = sampleRate ?? 0
        if rate == 0 { rate = codec.sampleRates?.max() ?? domains.sampleRates.max() ?? 0 }
        var count = channels ?? 0
        if count == 0 { count = codec.maxChannels ?? 0 }
        let caps = codec.bitrateCaps ?? []
        guard let cap = caps.filter({ ($0.minRate ?? 0) <= rate }).max(by: { ($0.minRate ?? 0) < ($1.minRate ?? 0) })
        else { return 0 }
        let limits = [cap.total ?? 0, (cap.perChannel ?? 0) * count, (cap.perSample ?? 0) * rate * count]
        return limits.filter { $0 > 0 }.min() ?? 0
    }

    /// audioBitrates lists the bitrate choices for the codec: its fixed list, else the domain list
    /// under the cap. Lossless codecs and codecs with no bitrate rules get none.
    public func audioBitrates(for codec: Codec, sampleRate: Int?, channels: Int?) -> [Int] {
        if let fixed = codec.bitrates, !fixed.isEmpty { return fixed }
        if codec.lossless == true || (codec.bitrateCaps ?? []).isEmpty { return [] }
        let cap = maxAudioBitrate(for: codec, sampleRate: sampleRate, channels: channels)
        return domains.audioBitrates.filter { cap == 0 || $0 <= cap }
    }

    /// defaultCodec returns the first available codec of a kind the container carries, or the
    /// first one when none is available.
    public func defaultCodec(_ kind: String, in container: Container) -> Codec? {
        let list = kind == "video" ? videoCodecs(in: container) : audioCodecs(in: container)
        return list.first { $0.available } ?? list.first
    }

    /// videoSettings returns video settings for codec in container, keeping what carries over from
    /// old: the fit box or frame size, the frame rate, the rate control, the quality, and the
    /// bitrate. Codec-specific settings such as profile, level, and tag start over.
    public func videoSettings(_ codec: Codec, in container: Container, keeping old: Preset.Video) -> Preset.Video {
        var v = Preset.Video(codec: codec.id)
        let tied = Set((codec.modes ?? []).compactMap(\.profile))
        if !tied.isEmpty {
            v.profile = (codec.profiles ?? []).first { !tied.contains($0.id) }?.id ?? codec.profiles?.first?.id
        }
        if !isFixed(codec, profile: v.profile, in: container) {
            v.maxWidth = old.maxWidth
            v.maxHeight = old.maxHeight
            v.width = old.width
            v.height = old.height
        }
        if let fps = old.fps, frameRates(for: codec).contains(where: { abs($0.fps - fps) < 0.01 }),
           !modes(for: codec, profile: v.profile, in: container).contains(where: { $0.fps != nil }) {
            v.fps = fps
        }
        let controls = codec.rateControls ?? []
        guard !controls.isEmpty else { return v }
        let preferred = [old.rateControl, RateControl.quality, RateControl.vbr].compactMap { $0 }
        let rc = preferred.first { controls.contains($0) } ?? controls[0]
        return withRateControl(v, rc, keeping: old)
    }

    /// withRateControl returns v under rate control rc, with the quality or bitrate it needs. A
    /// two-pass encode with no bitrate stays that way, since it aims for a target size.
    public func withRateControl(_ v: Preset.Video, _ rc: String, keeping old: Preset.Video) -> Preset.Video {
        var v = v
        v.rateControl = rc
        switch rc {
        case RateControl.quality:
            v.quality = old.quality ?? QualityLevel.high
            v.bitrate = nil
        case RateControl.twopass where old.rateControl == RateControl.twopass && old.bitrate == nil:
            v.quality = nil
            v.bitrate = nil
        default:
            v.quality = nil
            v.bitrate = old.bitrate ?? Catalog.defaultVideoBitrate
        }
        return v
    }

    /// audioSettings returns audio settings for codec in container, keeping the sample rate,
    /// channels, bitrate, and bit depth from old where the codec and container take them.
    public func audioSettings(_ codec: Codec, in container: Container, keeping old: Preset.Audio) -> Preset.Audio {
        var a = Preset.Audio(codec: codec.id)
        let rates = sampleRates(for: codec, in: container)
        if let rate = old.sampleRate, rates.contains(rate) {
            a.sampleRate = rate
        } else if container.sampleRates?.isEmpty == false {
            a.sampleRate = rates.max()
        }
        if let fixed = container.channels {
            a.channels = fixed
        } else if let count = old.channels, channelLayouts(for: codec, in: container).contains(where: { $0.count == count }) {
            a.channels = count
        }
        let choices = audioBitrates(for: codec, sampleRate: a.sampleRate, channels: a.channels)
        if let fixed = codec.bitrates, !fixed.isEmpty {
            a.bitrate = old.bitrate.flatMap { fixed.contains($0) ? $0 : nil } ?? fixed.max()
        } else if !choices.isEmpty {
            let cap = maxAudioBitrate(for: codec, sampleRate: a.sampleRate, channels: a.channels)
            let want = old.bitrate ?? Catalog.defaultAudioBitrate
            a.bitrate = cap == 0 || want <= cap ? want : choices.last
        }
        if let depth = old.bitDepth, codec.bitDepths?.contains(depth) == true { a.bitDepth = depth }
        return a
    }

    /// settingVideoCodec returns p with its video codec changed to id and the settings that depend
    /// on the codec started over.
    public func settingVideoCodec(_ p: Preset, to id: String) -> Preset {
        var p = p
        if id == copyCodec || id.isEmpty {
            p.video = Preset.Video(codec: id)
        } else if let c = codec(id: id), let box = container(id: p.container) {
            p.video = videoSettings(c, in: box, keeping: p.video)
        }
        return p
    }

    /// settingAudioCodec returns p with its audio codec changed to id and the settings that depend
    /// on the codec started over.
    public func settingAudioCodec(_ p: Preset, to id: String) -> Preset {
        var p = p
        if id == copyCodec || id.isEmpty {
            p.audio = Preset.Audio(codec: id)
        } else if let c = codec(id: id), let box = container(id: p.container) {
            p.audio = audioSettings(c, in: box, keeping: p.audio)
        }
        return p
    }

    /// retarget returns p moved into container c. The codecs reset to the container's first
    /// available ones, keeping the settings that carry over. Audio left off stays off in a video
    /// container.
    public func retarget(_ p: Preset, to c: Container) -> Preset {
        var out = p
        out.container = c.id
        out.ext = c.ext
        if c.kind == "video", let vc = defaultCodec("video", in: c) {
            let old = p.video.codec == copyCodec ? Preset.Video(codec: "") : p.video
            out.video = videoSettings(vc, in: c, keeping: old)
        } else {
            out.video = Preset.Video(codec: "")
        }
        if c.kind == "video" && p.audio.codec.isEmpty {
            out.audio = Preset.Audio(codec: "")
        } else if let ac = defaultCodec("audio", in: c) {
            let old = p.audio.codec == copyCodec ? Preset.Audio(codec: "") : p.audio
            out.audio = audioSettings(ac, in: c, keeping: old)
        } else {
            out.audio = Preset.Audio(codec: "")
        }
        return out
    }

    /// target returns the starting settings for container c at a resolution: its first available
    /// codecs at high quality, fit into the resolution's box when the format allows one.
    public func target(for c: Container, resolution: Resolution? = nil) -> Preset {
        let suffix = resolution.flatMap { $0.width == 0 ? nil : $0 }
        var p = Preset(id: [c.id, suffix?.id].compactMap { $0 }.joined(separator: "-"),
                       name: [c.name, suffix?.name].compactMap { $0 }.joined(separator: " "),
                       description: "", container: c.id, ext: c.ext,
                       video: Preset.Video(codec: ""), audio: Preset.Audio(codec: ""))
        if c.kind == "video", let vc = defaultCodec("video", in: c) {
            p.video = videoSettings(vc, in: c, keeping: Preset.Video(codec: ""))
        }
        if let ac = defaultCodec("audio", in: c) {
            p.audio = audioSettings(ac, in: c, keeping: Preset.Audio(codec: ""))
        }
        if let r = suffix { p = fitting(p, into: r) }
        p.description = summary(p)
        return p
    }

    /// fitting returns p with its video fit into resolution r, or unchanged when the format takes
    /// fixed frame shapes or the video is copied.
    public func fitting(_ p: Preset, into r: Resolution) -> Preset {
        guard let c = codec(id: p.video.codec), let box = container(id: p.container),
              !isFixed(c, profile: p.video.profile, in: box) else { return p }
        var p = p
        p.video.width = nil
        p.video.height = nil
        p.video.maxWidth = r.width == 0 ? nil : r.width
        p.video.maxHeight = r.height == 0 ? nil : r.height
        return p
    }

    /// resolution returns the ladder entry whose box matches the preset's fit box.
    public func resolution(of p: Preset) -> Resolution? {
        guard p.video.width == nil, p.video.height == nil else { return nil }
        return domains.resolutions.first {
            $0.width == (p.video.maxWidth ?? 0) && $0.height == (p.video.maxHeight ?? 0)
        }
    }

    /// summary describes the preset's container and codecs in a sentence.
    public func summary(_ p: Preset) -> String {
        let box = container(id: p.container)?.name ?? p.ext.uppercased()
        func name(_ id: String) -> String {
            id == copyCodec ? "copied" : (codec(id: id)?.name ?? id.uppercased())
        }
        switch (p.video.codec.isEmpty, p.audio.codec.isEmpty) {
        case (false, false): return "\(box) with \(name(p.video.codec)) video and \(name(p.audio.codec)) audio."
        case (false, true): return "\(box) with \(name(p.video.codec)) video and no audio."
        default: return "\(box) with \(name(p.audio.codec)) audio."
        }
    }
}
