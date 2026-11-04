import Foundation

/// Estimate formats the backend's size estimate for a row and works out the output frame the
/// chips show. The size itself comes from the backend's plan reply, where the formula and its
/// calibration live.
public enum Estimate {
    /// Range is an expected output size. A point estimate has low equal to high.
    public struct Range: Codable, Hashable, Sendable {
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
}

extension Double {
    /// nonZero returns the value, or fallback when the value is zero.
    func nonZero(or fallback: Double) -> Double { self == 0 ? fallback : self }
}
