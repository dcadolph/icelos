import Foundation

/// Estimate predicts an output size before any encode, from the probe and the settings. It is a
/// formula, never a measurement, so rows show it as approximate.
public enum Estimate {
    /// overhead is the container's share on top of the streams.
    static let overhead = 1.01
    /// assumedAudioBitrate stands in for a source audio stream, whose bitrate the probe omits.
    static let assumedAudioBitrate = 160_000.0
    /// defaultSampleRate is the sample rate assumed for uncompressed audio when none is set.
    static let defaultSampleRate = 48_000.0
    /// losslessShare is the size of FLAC and ALAC audio against the same audio uncompressed.
    static let losslessShare = 0.6
    /// areaExponent bends the size change of a resize: half the pixels costs more than half the
    /// bits.
    static let areaExponent = 0.75
    /// spread widens a quality range by this share on each side for what the ratio model leaves
    /// out: audio and container cost above the nominal bitrate and encoder variance, which put the
    /// calibration presets up to 8% outside the bare ratios.
    static let spread = 0.1
    /// ratios bound output against source video bitrate for the quality rate control, keyed by
    /// engine and codec ID and indexed by quality level less one. Measured 2026-11-03 on two 20 s
    /// fixtures from the wolf-eye-bird clip, one clean at 4.7 Mb/s and one with temporal grain at
    /// 17.4 Mb/s: each bound is the smaller or larger of the two. AV1 and VP9 are not measured yet
    /// and take 0.7 and 0.8 of software H.264.
    static let ratios: [Engine: [String: [Ratio]]] = {
        let h264 = [Ratio(0.1128, 0.5134), Ratio(0.0876, 0.1587), Ratio(0.0523, 0.0650)]
        return [
            .software: [
                "h264": h264,
                "hevc": [Ratio(0.0400, 0.1080), Ratio(0.0173, 0.0701), Ratio(0.0121, 0.0527)],
                "av1": h264.map { $0.scaled(0.7) },
                "vp9": h264.map { $0.scaled(0.8) },
            ],
            .hardware: [
                "h264": [Ratio(0.1741, 0.4687), Ratio(0.0517, 0.1112), Ratio(0.0257, 0.0909)],
                "hevc": [Ratio(0.0454, 0.1107), Ratio(0.0232, 0.0777), Ratio(0.0135, 0.0506)],
            ],
        ]
    }()

    /// Range is an expected output size. A point estimate has low equal to high.
    public struct Range: Equatable, Sendable {
        /// low is the smallest expected size in bytes.
        public var low: Int64
        /// high is the largest expected size in bytes.
        public var high: Int64

        /// Creates a range from low to high.
        public init(low: Int64, high: Int64) {
            self.low = low
            self.high = high
        }

        /// Creates a point estimate.
        public init(_ bytes: Int64) {
            self.init(low: bytes, high: bytes)
        }

        /// isPoint is true when the estimate is one size.
        public var isPoint: Bool { low == high }
    }

    /// Engine is the encoder family a quality ratio was measured on.
    public enum Engine: Hashable, Sendable {
        /// software is the ffmpeg software encoder.
        case software
        /// hardware is the VideoToolbox media engine.
        case hardware
    }

    /// Ratio is the low and high share of the source video bitrate an encode keeps.
    struct Ratio: Equatable, Sendable {
        /// low is the share on footage that compresses well.
        var low: Double
        /// high is the share on footage that does not.
        var high: Double

        /// Creates a ratio from low to high.
        init(_ low: Double, _ high: Double) {
            self.low = low
            self.high = high
        }

        /// scaled returns both bounds times factor.
        func scaled(_ factor: Double) -> Ratio { Ratio(low * factor, high * factor) }
    }

    /// bytes returns the expected output size of info converted with p, or nil when the formula
    /// has nothing to go on. Quality mode gives a range, copy, target size, and bitrate modes give
    /// a point. TargetBytes is the size the user asked for when p aims for one, and hardware is
    /// true when the media engine is on.
    public static func bytes(_ info: MediaInfo, _ p: Preset, targetBytes: Int64? = nil,
                             hardware: Bool = false) -> Range? {
        if p.video.codec == copyCodec && (p.audio.codec == copyCodec || info.audio == nil) {
            return info.size > 0 ? Range(info.size) : nil
        }
        if p.video.rateControl == RateControl.twopass, p.video.bitrate == nil {
            return (p.targetBytes ?? targetBytes).map(Range.init)
        }
        guard info.durationNs > 0 else { return nil }
        let seconds = Double(info.durationNs) / 1e9
        guard let video = videoBitrate(info, p, hardware: hardware),
              let audio = audioBitrate(info, p.audio) else { return nil }
        func size(_ videoBits: Double) -> Double { (videoBits + audio) * seconds / 8 * overhead }
        let low = size(video.low), high = size(video.high)
        guard low >= 1 else { return nil }
        if video.low == video.high { return Range(Int64(low.rounded())) }
        return Range(low: Int64((low * (1 - spread)).rounded()), high: Int64((high * (1 + spread)).rounded()))
    }

    /// videoBitrate returns the expected video bitrate in bits per second as a low and high
    /// bound, equal outside quality mode, zero when the output carries no video, or nil when the
    /// codec has no known ratio.
    static func videoBitrate(_ info: MediaInfo, _ p: Preset, hardware: Bool = false) -> Ratio? {
        guard !p.video.codec.isEmpty, let src = info.video else { return Ratio(0, 0) }
        let srcVideo = sourceVideoBitrate(info)
        if p.video.codec == copyCodec { return srcVideo.map { Ratio($0, $0) } }
        if p.video.rateControl != nil, p.video.rateControl != RateControl.quality {
            return p.video.bitrate.map { Ratio(Double($0), Double($0)) }
        }
        let engine: Engine = hardware && ratios[.hardware]?[p.video.codec] != nil ? .hardware : .software
        guard let table = ratios[engine]?[p.video.codec], let srcVideo else { return nil }
        let level = min(max(p.video.quality ?? QualityLevel.high, 1), table.count)
        return table[level - 1].scaled(srcVideo * areaScale(src, p.video))
    }

    /// sourceVideoBitrate is the source bitrate less an assumed audio share.
    static func sourceVideoBitrate(_ info: MediaInfo) -> Double? {
        guard let total = info.bitrate else { return nil }
        let audio = info.audio == nil ? 0 : min(assumedAudioBitrate, total / 2)
        return total - audio
    }

    /// areaScale is the bitrate factor of the output frame against the source frame.
    static func areaScale(_ src: MediaStream, _ v: Preset.Video) -> Double {
        guard let (sw, sh) = src.displaySize, let (ow, oh) = frame(src, v) else { return 1 }
        let ratio = Double(ow * oh) / Double(sw * sh)
        return pow(ratio, areaExponent)
    }

    /// audioBitrate returns the expected audio bitrate in bits per second, zero when the output
    /// carries no audio.
    static func audioBitrate(_ info: MediaInfo, _ a: Preset.Audio) -> Double? {
        guard !a.codec.isEmpty, let src = info.audio else { return 0 }
        if a.codec == copyCodec { return assumedAudioBitrate }
        let channels = Double(min(a.channels ?? Int.max, src.channels ?? 2))
        let pcm = Double(a.sampleRate ?? 0).nonZero(or: defaultSampleRate) * channels * Double(a.bitDepth ?? 16)
        switch a.codec {
        case _ where a.codec.hasPrefix("pcm_"):
            return pcm
        case "flac", "alac":
            return pcm * losslessShare
        default:
            return Double(a.bitrate ?? Catalog.defaultAudioBitrate)
        }
    }

    /// frame returns the output frame size for a source stream: the exact size when set, else
    /// the source fit into the box with the long side bounded by maxWidth, never enlarged and
    /// rounded down to even numbers, as the backend does.
    public static func frame(_ src: MediaStream, _ v: Preset.Video) -> (width: Int, height: Int)? {
        guard let (w, h) = src.displaySize else { return nil }
        if v.codec == copyCodec { return (w, h) }
        if let ew = v.width, let eh = v.height, ew > 0, eh > 0 { return (ew, eh) }
        var boxW = Double(v.maxWidth ?? 0).nonZero(or: .greatestFiniteMagnitude)
        var boxH = Double(v.maxHeight ?? 0).nonZero(or: .greatestFiniteMagnitude)
        if h > w { swap(&boxW, &boxH) }
        let scale = min(1, boxW / Double(w), boxH / Double(h))
        func even(_ n: Double) -> Int {
            let i = Int(n.rounded())
            return max(2, i - i % 2)
        }
        return (even(Double(w) * scale), even(Double(h) * scale))
    }

    /// text formats an estimate for a row, e.g. "≈ 48 MB, 71% smaller".
    public static func text(_ bytes: Int64, source: Int64?) -> String {
        let size = "≈ " + Format.bytes(bytes)
        guard let source, let saving = Format.saving(from: source, to: bytes) else { return size }
        return size + ", " + saving.prefix(1).lowercased() + saving.dropFirst()
    }

    /// text formats a range for a row, e.g. "≈ 8 to 25 MB, at least 40% smaller", with the
    /// saving taken from the high bound and shown only when the file shrinks.
    public static func text(_ r: Range, source: Int64?) -> String {
        let low = Format.bytes(r.low), high = Format.bytes(r.high)
        guard !r.isPoint, low != high else { return text(r.high, source: source) }
        var size = "≈ \(low) to \(high)"
        if let space = high.lastIndex(where: \.isWhitespace), low.hasSuffix(high[space...]) {
            size = "≈ \(low[..<low.index(low.endIndex, offsetBy: -high[space...].count)]) to \(high)"
        }
        guard let source, let saving = Format.saving(from: source, to: r.high),
              saving.hasSuffix("smaller") else { return size }
        return size + ", at least " + saving
    }

    /// parseSize reads a size such as "25MB", "8 MiB", or "1.5g" into bytes, as the backend
    /// does. A bare number is megabytes.
    public static func parseSize(_ s: String) -> Int64? {
        let units: [(String, Double)] = [
            ("gib", 1_073_741_824), ("mib", 1_048_576), ("kib", 1024),
            ("gb", 1e9), ("mb", 1e6), ("kb", 1e3),
            ("g", 1e9), ("m", 1e6), ("k", 1e3), ("b", 1),
        ]
        var text = s.lowercased().trimmingCharacters(in: .whitespaces)
        var mult = 1e6
        if let (suffix, bytes) = units.first(where: { text.hasSuffix($0.0) }) {
            text = String(text.dropLast(suffix.count)).trimmingCharacters(in: .whitespaces)
            mult = bytes
        }
        guard let n = Double(text).map({ $0 * mult }), n >= 1, n.isFinite, n < Double(Int64.max / 2) else {
            return nil
        }
        return Int64(n)
    }
}

extension Double {
    /// nonZero returns the value, or fallback when the value is zero.
    func nonZero(or fallback: Double) -> Double { self == 0 ? fallback : self }
}
