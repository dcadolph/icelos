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
    /// ratios are output against source video bitrate for the quality rate control, keyed by
    /// codec ID and indexed by quality level less one.
    static let ratios: [String: [Double]] = {
        let h264 = [0.55, 0.35, 0.2]
        return [
            "h264": h264,
            "hevc": h264.map { $0 * 0.7 },
            "av1": h264.map { $0 * 0.7 },
            "vp9": h264.map { $0 * 0.8 },
        ]
    }()
    /// bytes returns the expected output size of info converted with p, or nil when the formula
    /// has nothing to go on. TargetBytes is the size the user asked for when p aims for one.
    public static func bytes(_ info: MediaInfo, _ p: Preset, targetBytes: Int64? = nil) -> Int64? {
        if p.video.codec == copyCodec && (p.audio.codec == copyCodec || info.audio == nil) {
            return info.size > 0 ? info.size : nil
        }
        if p.video.rateControl == RateControl.twopass, p.video.bitrate == nil {
            return p.targetBytes ?? targetBytes
        }
        guard info.durationNs > 0 else { return nil }
        let seconds = Double(info.durationNs) / 1e9
        guard let video = videoBitrate(info, p),
              let audio = audioBitrate(info, p.audio) else { return nil }
        let total = (video + audio) * seconds / 8 * overhead
        return total >= 1 ? Int64(total.rounded()) : nil
    }

    /// videoBitrate returns the expected video bitrate in bits per second, zero when the output
    /// carries no video, or nil when the codec has no known ratio.
    static func videoBitrate(_ info: MediaInfo, _ p: Preset) -> Double? {
        guard !p.video.codec.isEmpty, let src = info.video else { return 0 }
        let srcVideo = sourceVideoBitrate(info)
        if p.video.codec == copyCodec { return srcVideo }
        if p.video.rateControl != nil, p.video.rateControl != RateControl.quality {
            return p.video.bitrate.map(Double.init)
        }
        guard let table = ratios[p.video.codec], let srcVideo else { return nil }
        let level = min(max(p.video.quality ?? QualityLevel.high, 1), table.count)
        return srcVideo * table[level - 1] * areaScale(src, p.video)
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
