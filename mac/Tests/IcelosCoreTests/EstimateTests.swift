import Foundation
import Testing

@testable import IcelosCore

/// builtins decodes the captured built-in presets by ID.
func builtins() throws -> [String: Preset] {
    let presets = try #require(try JSONDecoder().decode(Message.self, from: Data(presetsFixture.utf8)).presets)
    return Dictionary(uniqueKeysWithValues: presets.map { ($0.id, $0) })
}

/// calibrationSource is the probe of a 90 s 1920 by 1080 H.264 clip at 16 Mb/s with AAC stereo
/// at 256 kb/s.
let calibrationSource = MediaInfo(format: "mov,mp4,m4a,3gp,3g2,mj2", durationNs: 90_000_000_000, size: 181_765_278,
                                  streams: [
                                      MediaStream(index: 0, kind: "video", codec: "h264", width: 1920, height: 1080),
                                      MediaStream(index: 1, kind: "audio", codec: "aac", channels: 2),
                                  ])

@Suite struct EstimateTests {
    @Test func frameFitsTheBoxLikeTheBackend() throws {
        let box = Preset.Video(codec: "h264", maxWidth: 1920, maxHeight: 1080)
        let cases: [(MediaStream, Int, Int)] = [
            (MediaStream(index: 0, kind: "video", width: 3840, height: 2160), 1920, 1080),
            (MediaStream(index: 0, kind: "video", width: 2160, height: 3840), 1080, 1920),
            (MediaStream(index: 0, kind: "video", width: 1280, height: 720), 1280, 720),
            (MediaStream(index: 0, kind: "video", width: 4096, height: 1716), 1920, 804),
            (MediaStream(index: 0, kind: "video", width: 3840, height: 2160,
                         sideData: [MediaStream.SideData(rotation: -90)]), 1080, 1920),
        ]
        for (src, w, h) in cases {
            let got = try #require(Estimate.frame(src, box))
            #expect(got.width == w && got.height == h, "\(src.width ?? 0)x\(src.height ?? 0)")
        }
    }

    @Test func textShowsSizeAndSaving() {
        #expect(Estimate.text(48_000_000, source: 165_000_000) == "≈ 48 MB, 71% smaller")
        #expect(Estimate.text(48_000_000, source: nil) == "≈ 48 MB")
        #expect(Estimate.text(48_000_000, source: 48_000_000) == "≈ 48 MB, same size")
    }

    @Test func rangeTextSharesTheUnitAndClaimsTheSmallestSaving() {
        let r = Estimate.Range(low: 8_000_000, high: 25_000_000)
        #expect(Estimate.text(r, source: 100_000_000) == "≈ 8 to 25 MB, at least 75% smaller")
        #expect(Estimate.text(r, source: nil) == "≈ 8 to 25 MB")
        #expect(Estimate.text(r, source: 20_000_000) == "≈ 8 to 25 MB")
        let mixed = Estimate.Range(low: 800_000, high: 2_100_000)
        #expect(Estimate.text(mixed, source: nil) == "≈ 800 KB to 2.1 MB")
        #expect(Estimate.text(Estimate.Range(48_000_000), source: nil) == "≈ 48 MB")
    }
}
