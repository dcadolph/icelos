import Foundation
import Testing

@testable import IcelosCore

/// builtins decodes the captured built-in presets by ID.
func builtins() throws -> [String: Preset] {
    let presets = try #require(try JSONDecoder().decode(Message.self, from: Data(presetsFixture.utf8)).presets)
    return Dictionary(uniqueKeysWithValues: presets.map { ($0.id, $0) })
}

/// calibrationSource is the probe of the 90 s calibration clip: 1920 by 1080 H.264 at 16 Mb/s
/// with AAC stereo at 256 kb/s, made from the wolf-eye-bird reference clip looped with light
/// temporal grain.
let calibrationSource = MediaInfo(format: "mov,mp4,m4a,3gp,3g2,mj2", durationNs: 90_000_000_000, size: 181_765_278,
                                  streams: [
                                      MediaStream(index: 0, kind: "video", codec: "h264", width: 1920, height: 1080),
                                      MediaStream(index: 1, kind: "audio", codec: "aac", channels: 2),
                                  ])

/// measured holds the output sizes of the calibration clip through `icelos convert` with the
/// media engine on (the app default) and a 25MB target, from the 2026-11-02 run. Apple, web, and
/// tiny are quality mode, which the 20 s fixtures below check as ranges.
let measured: [String: Int64] = [
    "apple": 11_248_510,
    "compatible": 100_588_297,
    "web": 11_202_596,
    "tiny": 2_742_951,
    "target-size": 24_364_457,
    "remux": 181_765_227,
    "audio": 2_173_256,
    "mp3": 2_155_527,
]

/// Fixture is a 20 s quality calibration clip with its preset output sizes.
struct Fixture: Sendable {
    /// name tells the fixtures apart in test output.
    var name: String
    /// info is the clip's probe.
    var info: MediaInfo
    /// software holds output sizes by preset ID with the media engine off.
    var software: [String: Int64]
    /// hardware holds output sizes by preset ID with the media engine on.
    var hardware: [String: Int64]

    /// sizes returns the measured sizes for an engine.
    func sizes(hardware hw: Bool) -> [String: Int64] { hw ? hardware : software }
}

/// clip returns the probe of a 20 s 1920 by 1080 H.264 clip with stereo AAC of size bytes.
func clip(size: Int64) -> MediaInfo {
    MediaInfo(format: "mov,mp4,m4a,3gp,3g2,mj2", durationNs: 20_000_000_000, size: size,
              streams: [
                  MediaStream(index: 0, kind: "video", codec: "h264", width: 1920, height: 1080),
                  MediaStream(index: 1, kind: "audio", codec: "aac", channels: 2),
              ])
}

/// qualityFixtures are the two 20 s clips from the wolf-eye-bird reference looped, encoded with
/// h264_videotoolbox at 16 Mb/s and AAC at 256 kb/s, clean and with `noise=alls=6:allf=t`, and
/// the 2026-11-03 sizes of the quality presets through `icelos serve` plans.
let qualityFixtures = [
    Fixture(name: "clean", info: clip(size: 11_653_883),
            software: ["apple": 1_724_081, "compatible": 1_698_516, "web": 1_331_200, "tiny": 605_143],
            hardware: ["apple": 1_754_605, "compatible": 2_394_222, "web": 1_599_243, "tiny": 603_763]),
    Fixture(name: "grainy", info: clip(size: 43_393_755),
            software: ["apple": 2_234_214, "compatible": 22_710_486, "web": 7_228_001, "tiny": 560_670],
            hardware: ["apple": 2_469_503, "compatible": 20_770_089, "web": 2_582_508, "tiny": 605_796]),
]

/// qualityPresets are the built-in presets that use the quality rate control.
let qualityPresets = ["apple", "compatible", "web", "tiny"]

/// keptRatio inverts the estimate formula: the share of the source video bitrate that an output
/// of bytes kept, for the same audio, frame, and overhead terms the estimate uses.
func keptRatio(_ bytes: Int64, _ info: MediaInfo, _ p: Preset) throws -> Double {
    let seconds = Double(info.durationNs) / 1e9
    let audio = try #require(Estimate.audioBitrate(info, p.audio))
    let src = try #require(Estimate.sourceVideoBitrate(info))
    let video = Double(bytes) * 8 / seconds / Estimate.overhead - audio
    return video / src / Estimate.areaScale(try #require(info.video), p.video)
}

@Suite struct EstimateTests {
    @Test(arguments: ["target-size", "remux", "audio", "mp3"])
    func pointPresetWithin35PercentOfMeasured(id: String) throws {
        let p = try #require(try builtins()[id])
        let want = try #require(measured[id])
        let got = try #require(Estimate.bytes(calibrationSource, p, targetBytes: 25_000_000))
        #expect(got.isPoint)
        let off = abs(Double(got.high) / Double(want) - 1)
        #expect(off <= 0.35, "estimate \(got.high) against measured \(want)")
    }

    @Test(arguments: qualityPresets, [false, true])
    func qualityRangeHoldsTheMeasuredSize(id: String, hardware: Bool) throws {
        let p = try #require(try builtins()[id])
        for fx in qualityFixtures {
            let want = try #require(fx.sizes(hardware: hardware)[id])
            let got = try #require(Estimate.bytes(fx.info, p, hardware: hardware))
            #expect(!got.isPoint)
            #expect(got.low <= want && want <= got.high, "\(fx.name): \(want) outside \(got.low) to \(got.high)")
        }
    }

    @Test(arguments: qualityPresets, [false, true])
    func qualityBoundsWithin2xOfNearestMeasurement(id: String, hardware: Bool) throws {
        let p = try #require(try builtins()[id])
        let kept = try qualityFixtures.map { fx in
            try keptRatio(try #require(fx.sizes(hardware: hardware)[id]), fx.info, p)
        }
        let bounds = try #require(Estimate.videoBitrate(calibrationSource, p, hardware: hardware))
        let src = try #require(Estimate.sourceVideoBitrate(calibrationSource))
        let area = Estimate.areaScale(try #require(calibrationSource.video), p.video)
        for bound in [bounds.low, bounds.high].map({ $0 / src / area }) {
            let nearest = try #require(kept.map { abs(log(bound / $0)) }.min())
            #expect(nearest <= log(2), "bound \(bound) against measured \(kept)")
        }
    }

    @Test func hardwareColumnOnlyForMediaEngineCodecs() throws {
        let web = try #require(try builtins()["web"])
        let sw = try #require(Estimate.videoBitrate(calibrationSource, web, hardware: false))
        let hw = try #require(Estimate.videoBitrate(calibrationSource, web, hardware: true))
        #expect(sw != hw)
        var av1 = web
        av1.video.codec = "av1"
        #expect(Estimate.videoBitrate(calibrationSource, av1, hardware: true)
            == Estimate.videoBitrate(calibrationSource, av1, hardware: false))
    }

    @Test func copyIsTheSourceSize() throws {
        let p = try #require(try builtins()["remux"])
        #expect(Estimate.bytes(calibrationSource, p) == Estimate.Range(calibrationSource.size))
    }

    @Test func targetSizeWithoutTargetHasNoEstimate() throws {
        let p = try #require(try builtins()["target-size"])
        #expect(Estimate.bytes(calibrationSource, p) == nil)
        var sized = p
        sized.targetBytes = 8_000_000
        #expect(Estimate.bytes(calibrationSource, sized, targetBytes: 25_000_000) == Estimate.Range(8_000_000))
    }

    @Test func bitrateModeIsBitrateTimesDuration() throws {
        var p = try #require(try builtins()["compatible"])
        p.video.rateControl = RateControl.cbr
        p.video.bitrate = 4_000_000
        let bitsPerSecond: Double = 4_000_000 + 160_000
        let want = Int64((bitsPerSecond * 90 / 8 * 1.01).rounded())
        #expect(Estimate.bytes(calibrationSource, p) == Estimate.Range(want))
    }

    @Test func uncompressedAudioIsExact() throws {
        var p = try #require(try builtins()["audio"])
        p.audio = Preset.Audio(codec: "pcm_s16le", sampleRate: 44_100)
        let bitsPerSecond: Double = 44_100 * 2 * 16
        let want = Int64((bitsPerSecond * 90 / 8 * 1.01).rounded())
        #expect(Estimate.bytes(calibrationSource, p) == Estimate.Range(want))
    }

    @Test func unknownCodecOrDurationHasNoEstimate() throws {
        var p = try #require(try builtins()["compatible"])
        p.video.codec = "mpeg2video"
        #expect(Estimate.bytes(calibrationSource, p) == nil)
        var short = calibrationSource
        short.durationNs = 0
        #expect(Estimate.bytes(short, try #require(try builtins()["web"])) == nil)
    }

    @Test func resizeShrinksTheEstimate() throws {
        let p = try #require(try builtins()["compatible"])
        var uhd = calibrationSource
        uhd.streams?[0].width = 3840
        uhd.streams?[0].height = 2160
        let full = try #require(Estimate.videoBitrate(calibrationSource, p))
        let scaled = try #require(Estimate.videoBitrate(uhd, p))
        #expect(abs(scaled.low / full.low - pow(0.25, Estimate.areaExponent)) < 1e-9)
        #expect(abs(scaled.high / full.high - pow(0.25, Estimate.areaExponent)) < 1e-9)
    }

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

    @Test func parseSizeMatchesTheBackend() {
        #expect(Estimate.parseSize("25MB") == 25_000_000)
        #expect(Estimate.parseSize("8 MiB") == 8_388_608)
        #expect(Estimate.parseSize("1.5g") == 1_500_000_000)
        #expect(Estimate.parseSize("10") == 10_000_000)
        #expect(Estimate.parseSize("lots") == nil)
        #expect(Estimate.parseSize("0") == nil)
    }
}
