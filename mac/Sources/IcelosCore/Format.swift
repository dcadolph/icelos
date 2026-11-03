import Foundation

/// Format turns backend numbers into short display strings.
public enum Format {
    /// bytes formats a byte count, e.g. "412 MB".
    public static func bytes(_ n: Int64) -> String {
        ByteCountFormatter.string(fromByteCount: n, countStyle: .file)
    }

    /// duration formats nanoseconds as "m:ss" or "h:mm:ss".
    public static func duration(ns: Int64) -> String {
        let total = max(0, Int((Double(ns) / 1e9).rounded()))
        let h = total / 3600, m = (total % 3600) / 60, s = total % 60
        if h > 0 { return String(format: "%d:%02d:%02d", h, m, s) }
        return String(format: "%d:%02d", m, s)
    }

    /// summary describes a probed file, e.g. "1920×1080 · HEVC · 1:23 · 412 MB".
    public static func summary(_ info: MediaInfo) -> String {
        var parts: [String] = []
        if let v = info.video, let (w, h) = v.displaySize {
            parts.append("\(w)×\(h)")
            if let c = v.codec { parts.append(c.uppercased()) }
        } else if let a = info.audio, let c = a.codec {
            parts.append(c.uppercased())
        }
        if info.durationNs > 0 { parts.append(duration(ns: info.durationNs)) }
        if info.size > 0 { parts.append(bytes(info.size)) }
        return parts.joined(separator: " · ")
    }

    /// encoder names the encoder for people, e.g. "Media Engine" for VideoToolbox.
    public static func encoder(_ progress: JobProgress) -> String? {
        guard let name = progress.encoder, !name.isEmpty else { return nil }
        if progress.hardware { return "Media Engine" }
        switch name {
        case "libx264": return "Software H.264"
        case "libx265": return "Software HEVC"
        default: return name
        }
    }

    /// saving describes the size change between two files, e.g. "62% smaller".
    public static func saving(from source: Int64, to output: Int64) -> String? {
        guard source > 0, output > 0 else { return nil }
        let ratio = Double(output) / Double(source)
        if ratio < 0.995 { return "\(Int(((1 - ratio) * 100).rounded()))% smaller" }
        if ratio > 1.005 { return "\(Int(((ratio - 1) * 100).rounded()))% larger" }
        return "Same size"
    }
}
