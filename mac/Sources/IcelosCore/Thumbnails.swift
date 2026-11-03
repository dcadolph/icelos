import AVFoundation
import CoreGraphics

/// Thumbnails makes the small frame a file row shows, through AVFoundation rather than ffmpeg.
public enum Thumbnails {
    /// size is the largest image made: the row tile at twice its 96 by 54 point size.
    public static let size = CGSize(width: 192, height: 108)
    /// seconds is where in the file the frame is taken, or halfway through a shorter file.
    public static let seconds = 2.0

    /// image returns a frame of the video at url, or nil when AVFoundation cannot read it. It
    /// runs off the main actor.
    public static func image(for url: URL) async -> CGImage? {
        let asset = AVURLAsset(url: url)
        let generator = AVAssetImageGenerator(asset: asset)
        generator.appliesPreferredTrackTransform = true
        generator.maximumSize = size
        let tolerance = CMTime(seconds: 0.5, preferredTimescale: 600)
        generator.requestedTimeToleranceBefore = tolerance
        generator.requestedTimeToleranceAfter = tolerance
        guard let duration = try? await asset.load(.duration), duration.seconds > 0 else { return nil }
        let at = CMTime(seconds: min(seconds, duration.seconds / 2), preferredTimescale: 600)
        return try? await generator.image(at: at).image
    }
}
