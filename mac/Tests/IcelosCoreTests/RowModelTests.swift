import Foundation
import Testing

@testable import IcelosCore

/// catalog decodes the captured catalog reply.
private func catalog() throws -> Catalog {
    try #require(try JSONDecoder().decode(Message.self, from: Data(catalogFixture.utf8)).catalog)
}

/// progress returns a progress report at percent.
private func progress(_ percent: Double, passes: Int = 1, speed: Double = 0, etaNs: Int64 = 0) -> JobProgress {
    JobProgress(percent: percent, pass: 1, passes: passes, elapsedNs: 0, speed: speed, fps: 0, etaNs: etaNs,
                encoder: "hevc_videotoolbox", hardware: true)
}

/// item returns a probed calibration clip in phase, converted with p.
private func item(_ phase: Phase, preset p: Preset) -> Item {
    var it = Item(id: UUID(), url: URL(fileURLWithPath: "/x/clip.mov"), info: calibrationSource)
    func status(_ state: JobState, _ pr: JobProgress = progress(0), error: String? = nil) -> JobStatus {
        JobStatus(id: "1", job: Job(input: "/x/clip.mov", output: "/x/clip.mp4", preset: p, hardware: true),
                  state: state, progress: pr, error: error)
    }
    switch phase {
    case .pending: break
    case .queued: it.submitted = true
    case .running: it.submitted = true; it.status = status(.running, progress(0.47, passes: 2, speed: 1.5, etaNs: 32_000_000_000))
    case .done: it.submitted = true; it.status = status(.done); it.outputSize = 48_000_000
    case .failed: it.submitted = true; it.status = status(.failed, error: "encode failed: no space")
    case .canceled: it.submitted = true; it.status = status(.canceled)
    }
    return it
}

@Suite struct RowModelTests {
    @Test(arguments: ["apple", "compatible", "web", "tiny", "target-size", "remux", "audio", "mp3"])
    func everyPendingRowSaysWhatItWillMake(id: String) throws {
        let p = try #require(try builtins()[id])
        let row = RowModel(item: item(.pending, preset: p), windowSettings: p, catalog: try catalog(), targetSize: "25MB")
        let want: [String: [String]] = [
            "apple": ["MP4", "1920×1080", "HEVC (H.265)"],
            "compatible": ["MP4", "1920×1080", "H.264"],
            "web": ["MP4", "1920×1080", "H.264"],
            "tiny": ["MP4", "1280×720", "HEVC (H.265)"],
            "target-size": ["MP4", "1920×1080", "H.264"],
            "remux": ["MP4", "1920×1080", "Copy"],
            "audio": ["M4A", "AAC", "192 kb/s"],
            "mp3": ["MP3", "MP3", "192 kb/s"],
        ]
        #expect(row.chips == want[id])
        let estimate = try #require(row.estimate)
        #expect(estimate.hasPrefix("≈ "))
        #expect(row.badge == RowModel.Badge(text: "Ready", tone: .muted))
        #expect(row.actions == [.remove])
        #expect(row.media == .video)
    }

    @Test func sixPhasesGiveSixDistinctBadges() throws {
        let p = try #require(try builtins()["apple"])
        let phases: [Phase] = [.pending, .queued, .running, .done, .failed, .canceled]
        let badges = phases.map { RowModel(item: item($0, preset: p), windowSettings: p, catalog: nil).badge }
        #expect(Set(badges).count == 6)
        #expect(badges.map(\.text) == ["Ready", "Queued", "47%", "Done", "Failed", "Canceled"])
        #expect(badges.map(\.tone) == [.muted, .muted, .accent, .success, .danger, .muted])
    }

    @Test func phasesPickTheTargetLineAndActions() throws {
        let p = try #require(try builtins()["apple"])
        let running = RowModel(item: item(.running, preset: p), windowSettings: p, catalog: nil)
        #expect(running.progress == 0.47)
        #expect(running.detail == "Pass 1 of 2 · 1.5× · 0:32 left · Media Engine")
        #expect(running.estimate == nil)
        #expect(running.actions == [.cancel])

        let done = RowModel(item: item(.done, preset: p), windowSettings: p, catalog: nil)
        #expect(done.detail == "clip.mp4 · 48 MB · 74% smaller")
        #expect(done.detailTone == .success)
        #expect(done.actions == [.reveal, .remove])

        let failed = RowModel(item: item(.failed, preset: p), windowSettings: p, catalog: nil)
        #expect(failed.detail == "encode failed: no space")
        #expect(failed.detailTone == .danger)
        #expect(failed.actions == [.retry, .remove])

        let queued = RowModel(item: item(.queued, preset: p), windowSettings: p, catalog: nil)
        #expect(queued.estimate != nil)
        #expect(queued.actions == [.cancel])
    }

    @Test func unreadAndUnreadableFiles() throws {
        let p = try #require(try builtins()["apple"])
        var it = Item(id: UUID(), url: URL(fileURLWithPath: "/x/clip.mov"))
        let unread = RowModel(item: it, windowSettings: p, catalog: nil)
        #expect(unread.badge.text == "Reading")
        #expect(unread.media == .unknown)
        #expect(unread.estimate == nil)
        #expect(unread.chips == ["MP4", "HEVC"])

        it.probeError = "probe failed: bad file"
        let bad = RowModel(item: it, windowSettings: p, catalog: nil)
        #expect(bad.badge == RowModel.Badge(text: "Unreadable", tone: .danger))
        #expect(bad.source == "probe failed: bad file")
        #expect(bad.sourceTone == .danger)
        #expect(bad.actions == [.remove])
    }

    @Test func ownSettingsWinOverTheWindow() throws {
        let window = try #require(try builtins()["apple"])
        var it = item(.pending, preset: window)
        let mp3 = try #require(try builtins()["mp3"])
        it.settings = mp3
        let row = RowModel(item: it, windowSettings: window, catalog: try catalog())
        #expect(row.ownSettings)
        #expect(row.chips == ["MP3", "MP3", "192 kb/s"])
    }

    @Test func losslessChipFollowsWhatIsCopied() throws {
        let p = try #require(try builtins()["web"])
        var it = item(.pending, preset: p)
        let both = Copy(video: Verdict(copy: true), audio: Verdict(copy: true))
        let off = Copy(video: Verdict(copy: false, reason: "passthrough off"),
                       audio: Verdict(copy: false, reason: "passthrough off"))
        it.plan = Plan(copy: both, copied: off, lossless: false, hardware: true)
        let qualifies = RowModel(item: it, windowSettings: p, catalog: try catalog(), hardware: true)
        #expect(!qualifies.lossless, "copy true but copied false shows no chip")
        #expect(qualifies.estimate?.contains(" to ") == true, "an encode keeps the quality range")

        it.plan = Plan(copy: both, copied: both, lossless: true)
        let copies = RowModel(item: it, windowSettings: p, catalog: try catalog(), hardware: true)
        #expect(copies.lossless)
        let estimate = try #require(copies.estimate)
        #expect(!estimate.contains(" to "), "copied streams are a point estimate: \(estimate)")

        it.plan = nil
        #expect(!RowModel(item: it, windowSettings: p, catalog: nil).lossless, "no plan, no chip")
    }

    @Test func planHardwareDecidesTheEstimate() throws {
        let p = try #require(try builtins()["web"])
        var it = item(.pending, preset: p)
        let encode = Copy(video: Verdict(copy: false, reason: "codec"), audio: Verdict(copy: false, reason: "codec"))
        let software = RowModel(item: it, windowSettings: p, catalog: nil, hardware: false).estimate
        let hardware = RowModel(item: it, windowSettings: p, catalog: nil, hardware: true).estimate
        #expect(software != hardware)
        it.plan = Plan(copy: encode, copied: encode, lossless: false, hardware: false)
        #expect(RowModel(item: it, windowSettings: p, catalog: nil, hardware: true).estimate == software,
                "the plan's encoder wins over the switch")
    }

    @Test func copyNoteSaysWhatCopies() {
        #expect(CopyNote.text(nil, passthrough: true) == "Select a file to see which of its streams copy.")
        let off = Verdict(copy: false, reason: "passthrough off")
        let both = Copy(video: Verdict(copy: true), audio: Verdict(copy: true))
        #expect(CopyNote.text(Plan(copy: both, copied: both, lossless: true), passthrough: true)
                == "Video and audio copy as they are. Nothing is re-encoded.")
        #expect(CopyNote.text(Plan(copy: both, copied: Copy(video: off, audio: off), lossless: false), passthrough: false)
                == "Video is encoded: it already matches, so turning this on copies it. Audio is encoded: it already matches, so turning this on copies it.")
        let resize = Copy(video: Verdict(copy: false, reason: "resize"), audio: Verdict(copy: true))
        #expect(CopyNote.text(Plan(copy: resize, copied: resize, lossless: false), passthrough: true)
                == "Video is encoded: the frame size changes. Audio copies.")
    }

    @Test func audioSourceShowsAWaveform() throws {
        let p = try #require(try builtins()["mp3"])
        var it = Item(id: UUID(), url: URL(fileURLWithPath: "/x/song.flac"))
        it.info = MediaInfo(format: "flac", durationNs: 1_000_000_000, size: 100, streams: [
            MediaStream(index: 0, kind: "audio", codec: "flac", channels: 2),
            MediaStream(index: 1, kind: "video", codec: "mjpeg", width: 600, height: 600,
                        disposition: MediaStream.Disposition(attachedPic: 1)),
        ])
        let row = RowModel(item: it, windowSettings: p, catalog: nil)
        #expect(row.media == .audio)
        #expect(row.source == "FLAC · 0:01 · 100 bytes")
    }
}
