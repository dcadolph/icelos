import Foundation
import Testing

@testable import IcelosCore

/// statusJSON returns a backend job status line for job id in state.
private func statusLine(type: String, reply: String? = nil, id: String, state: String,
                        percent: Double = 0, output: String = "/x/clip.mp4") -> String {
    let replyField = reply.map { #""reply":"\#($0)","# } ?? ""
    return #"{"type":"\#(type)",\#(replyField)"status":{"id":"\#(id)","job":{"input":"/x/clip.mov","output":"\#(output)","preset":{"id":"web","name":"Web and sharing","description":"d","ext":"mp4","video":{"codec":"h264","maxWidth":1920,"maxHeight":1080,"quality":2},"audio":{"codec":"aac","bitrate":128000,"channels":2}},"hardware":false,"overwrite":false},"state":"\#(state)","progress":{"percent":\#(percent),"pass":1,"passes":1,"elapsedNs":0,"speed":2,"fps":0,"etaNs":0,"encoder":"libx264","hardware":false}}}"#
}

/// decode parses one backend line.
private func decode(_ line: String) throws -> Message {
    try JSONDecoder().decode(Message.self, from: Data(line.utf8))
}

/// presetsLine is a minimal presets reply.
private let presetsLine = #"{"type":"presets","reply":"presets","presets":[{"id":"apple","name":"Apple devices","description":"d","ext":"mp4","video":{"codec":"hevc"},"audio":{"codec":"aac"}},{"id":"target-size","name":"Fit a size","description":"d","ext":"mp4","video":{"codec":"h264"},"audio":{"codec":"aac"}},{"id":"audio","name":"Audio only","description":"d","ext":"m4a","video":{"codec":""},"audio":{"codec":"aac"}}]}"#

/// Harness collects sent requests.
@MainActor
private final class Harness {
    /// sent holds every request in order.
    var sent: [Request] = []
    /// model is the converter under test.
    lazy var model = Converter(send: { [unowned self] in self.sent.append($0) }, fileSize: { _ in 40 })

    /// load feeds the model the captured presets and catalog replies.
    func load() throws {
        model.handle(try JSONDecoder().decode(Message.self, from: Data(catalogFixture.utf8)))
        model.handle(try JSONDecoder().decode(Message.self, from: Data(presetsFixture.utf8)))
    }
}

@MainActor
@Suite struct ConverterTests {
    @Test func readyAsksForPresetsAndCapabilities() throws {
        let h = Harness()
        h.model.handle(try decode(#"{"type":"ready","version":"v1.0.0"}"#))
        #expect(h.model.version == "v1.0.0")
        #expect(h.sent.map(\.op) == ["presets", "capabilities", "catalog"])
        h.model.handle(try decode(presetsLine))
        #expect(h.model.presets.map(\.id) == ["apple", "target-size", "audio"])
        #expect(h.model.presets[2].isAudioOnly)
        h.model.handle(try decode(#"{"type":"capabilities","reply":"capabilities","encoders":["libx264"]}"#))
        #expect(!h.model.hasHardware)
        #expect(!h.model.hardware)
    }

    @Test func unknownSelectedPresetFallsBackToFirst() throws {
        let h = Harness()
        h.model.presetID = "gone"
        h.model.handle(try decode(presetsLine))
        #expect(h.model.presetID == "apple")
    }

    @Test func fullJobLifecycleWithEarlyEvents() throws {
        let h = Harness()
        h.model.handle(try decode(presetsLine))
        h.model.presetID = "target-size"
        h.model.targetSize = "8MB"
        h.model.outputFolder = URL(fileURLWithPath: "/out")
        h.model.add(urls: [URL(fileURLWithPath: "/x/clip.mov")])
        let item = h.model.items[0]
        #expect(item.phase == .pending)
        #expect(h.sent.last == Request(id: "probe:" + item.id.uuidString, op: "probe", path: "/x/clip.mov"))

        h.model.handle(try decode(#"{"type":"probe","reply":"probe:\#(item.id.uuidString)","info":{"format":"mov","durationNs":83000000000,"size":100,"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":3840,"height":2160,"side_data_list":null}]}}"#))
        #expect(Format.summary(h.model.items[0].info!) == "3840×2160 · HEVC · 1:23 · 100 bytes")
        #expect(h.model.pendingCount == 1)

        h.model.convertAll()
        #expect(h.sent.last == Request(id: "add:" + item.id.uuidString, op: "add", input: "/x/clip.mov",
                                       preset: "target-size", size: "8MB", hardware: true, outDir: "/out"))
        #expect(h.model.items[0].phase == .queued)
        #expect(h.model.isBusy)

        h.model.handle(try decode(statusLine(type: "job", id: "1", state: "running", percent: 0.4)))
        h.model.handle(try decode(statusLine(type: "add", reply: "add:" + item.id.uuidString, id: "1", state: "queued")))
        #expect(h.model.items[0].phase == .running)
        #expect(h.model.overallProgress == 0.4)

        h.model.handle(try decode(statusLine(type: "job", id: "1", state: "done", percent: 1)))
        #expect(h.model.items[0].phase == .done)
        #expect(h.model.items[0].outputSize == 40)
        #expect(h.model.savedBytes == 60)
        #expect(h.model.overallProgress == nil)
        #expect(h.model.items[0].outputURL?.path == "/x/clip.mp4")

        h.model.handle(try decode(statusLine(type: "job", id: "1", state: "running", percent: 0.9)))
        #expect(h.model.items[0].phase == .done, "a late progress event must not reopen a finished job")

        h.model.clearFinished()
        #expect(h.model.items.isEmpty)
        #expect(h.sent.last?.op == "clear")
    }

    @Test func errorsLandOnTheRightItem() throws {
        let h = Harness()
        h.model.handle(try decode(presetsLine))
        h.model.add(urls: [URL(fileURLWithPath: "/x/a.mov"), URL(fileURLWithPath: "/x/b.mov")])
        let (a, b) = (h.model.items[0], h.model.items[1])
        h.model.handle(try decode(#"{"type":"error","reply":"probe:\#(a.id.uuidString)","error":"probe failed: bad file"}"#))
        #expect(h.model.items[0].errorMessage == "probe failed: bad file")
        #expect(h.model.pendingCount == 1)

        h.model.convertAll()
        #expect(h.sent.filter { $0.op == "add" }.count == 1, "unreadable files are not submitted")
        h.model.handle(try decode(#"{"type":"error","reply":"add:\#(b.id.uuidString)","error":"output exists"}"#))
        #expect(h.model.items[1].phase == .failed)

        h.model.retry(b.id)
        #expect(h.model.items[1].phase == .pending)
        #expect(h.model.items[1].info == b.info)

        h.model.handle(try decode(#"{"type":"error","error":"bad request"}"#))
        #expect(h.model.fault == "bad request")
    }

    @Test func cancelAndRemove() throws {
        let h = Harness()
        h.model.handle(try decode(presetsLine))
        h.model.add(urls: [URL(fileURLWithPath: "/x/clip.mov")])
        h.model.add(urls: [URL(fileURLWithPath: "/x/clip.mov")])
        #expect(h.model.items.count == 1, "a pending duplicate is skipped")
        let item = h.model.items[0]
        h.model.cancel(item.id)
        #expect(h.sent.last?.op == "probe", "a pending item has no job to cancel")
        h.model.convertAll()
        h.model.handle(try decode(statusLine(type: "add", reply: "add:" + item.id.uuidString, id: "7", state: "running")))
        h.model.remove(item.id)
        #expect(h.model.items.count == 1, "a running item cannot be removed")
        h.model.cancel(item.id)
        #expect(h.sent.last == Request(id: "cancel:" + item.id.uuidString, op: "cancel", job: "7"))
        h.model.handle(try decode(statusLine(type: "job", id: "7", state: "canceled")))
        h.model.remove(item.id)
        #expect(h.model.items.isEmpty)
    }

    @Test func backendExitFailsInFlightItems() throws {
        let h = Harness()
        h.model.handle(try decode(presetsLine))
        h.model.add(urls: [URL(fileURLWithPath: "/x/clip.mov")])
        h.model.convertAll()
        h.model.backendExited("boom")
        #expect(h.model.fault == "boom")
        #expect(h.model.items[0].phase == .failed)
        h.model.handle(try decode(#"{"type":"ready","version":"v2"}"#))
        #expect(h.model.fault == nil)
    }

    @Test func catalogReplyIsKept() throws {
        let h = Harness()
        try h.load()
        #expect(h.model.catalog?.containers.isEmpty == false)
        #expect(h.model.container?.id == "mp4")
        #expect(h.model.settings?.id == "apple")
        #expect(!h.model.isEdited)
    }

    @Test func changingContainerResetsCodecs() throws {
        let h = Harness()
        try h.load()
        h.model.setContainer("webm")
        var s = try #require(h.model.settings)
        #expect(s.container == "webm")
        #expect(s.ext == "webm")
        #expect(s.video.codec == "vp9", "webm's first allowed codec")
        #expect(s.audio.codec == "opus")
        #expect(s.video.maxWidth == 3840, "the fit box carries over")
        #expect(s.video.rateControl == "quality")
        #expect(h.model.isEdited)

        h.model.setContainer("mkv")
        s = try #require(h.model.settings)
        #expect(s.video.codec == "h264")
        #expect(s.audio.codec == "aac")

        h.model.settings?.audio = Preset.Audio(codec: "")
        h.model.setContainer("mov")
        #expect(h.model.settings?.audio.codec == "", "audio left off stays off in a video format")

        h.model.setContainer("mp3")
        s = try #require(h.model.settings)
        #expect(s.isAudioOnly)
        #expect(s.audio.codec == "mp3", "an audio format needs its audio back")
        #expect(s.audio.bitrate == 192_000)
    }

    @Test func fixedFormatsTakeNoFitBox() throws {
        let h = Harness()
        try h.load()
        let catalog = try #require(h.model.catalog)
        let dv = try #require(catalog.container(id: "dv"))
        let p1080 = try #require(catalog.domains.resolutions.first { $0.id == "1080p" })
        h.model.select(container: dv, resolution: p1080)
        let s = try #require(h.model.settings)
        #expect(h.model.presetID == "")
        #expect(s.video.codec == "dvvideo")
        #expect(s.video.maxWidth == nil && s.video.width == nil)
        #expect(s.video.rateControl == nil, "dvvideo has no rate control choice")
        #expect(s.audio.codec == "pcm_s16le")
        #expect(s.audio.sampleRate == 48000)
        #expect(s.audio.channels == 2)
        #expect(s.audio.bitrate == nil)
    }

    @Test func catalogSelectionSendsInlineSettings() throws {
        let h = Harness()
        try h.load()
        let catalog = try #require(h.model.catalog)
        let mp4 = try #require(catalog.container(id: "mp4"))
        h.model.select(container: mp4)
        #expect(h.model.settings?.id == "mp4")
        #expect(h.model.settings?.video.codec == "h264")
        #expect(h.model.settings?.video.quality == 1)
        h.model.settings = catalog.settingVideoCodec(h.model.settings!, to: "hevc")
        let p720 = try #require(catalog.domains.resolutions.first { $0.id == "720p" })
        h.model.select(container: mp4, resolution: p720)
        let s = try #require(h.model.settings)
        #expect(s.video.codec == "hevc", "a resolution on the same format keeps the codec")
        #expect(s.video.maxWidth == 1280 && s.video.maxHeight == 720)

        h.model.add(urls: [URL(fileURLWithPath: "/x/clip.mov")])
        h.model.convertAll()
        let add = try #require(h.sent.last)
        #expect(add.op == "add")
        #expect(add.preset == nil)
        #expect(add.presetBody == s)
    }

    @Test func savingCustomPresetSendsSave() throws {
        let h = Harness()
        try h.load()
        var edited = try #require(h.model.settings)
        edited.audio.bitrate = 256_000
        h.model.settings = edited
        h.model.savePreset(edited, name: "  My Phone!  ")
        let save = try #require(h.sent.last)
        #expect(save.op == "preset.save")
        let body = try #require(save.presetBody)
        #expect(body.id == "my-phone")
        #expect(body.name == "My Phone!")
        #expect(body.audio.bitrate == 256_000)
        #expect(body.description == "MP4 with HEVC (H.265) video and AAC audio.")

        var stored = body
        stored.custom = true
        let presets = try #require(h.model.presets) + [stored]
        h.model.handle(Message(type: "preset.save", reply: "preset.save", presets: presets))
        #expect(h.model.presetID == "my-phone")
        #expect(h.model.settings == stored)
        #expect(h.model.customPresets.map(\.id) == ["my-phone"])
        #expect(!h.model.isEdited)

        h.model.deletePreset("my-phone")
        #expect(h.sent.last == Request(id: "preset.delete", op: "preset.delete", preset: "my-phone"))
        h.model.handle(Message(type: "preset.delete", reply: "preset.delete", presets: Array(presets.dropLast())))
        #expect(h.model.presetID == "apple", "a deleted selection falls back to the first preset")

        h.model.savePreset(edited, name: "Apple")
        #expect(h.sent.last?.presetBody?.id == "apple-custom", "built-in IDs are never overwritten")
        h.model.handle(try decode(#"{"type":"error","reply":"preset.save","error":"preset exists"}"#))
        #expect(h.model.presetError == "preset exists")
        #expect(h.model.fault == nil)
    }

    @Test func perFileOverrideSendsInlineBody() throws {
        let h = Harness()
        try h.load()
        h.model.add(urls: [URL(fileURLWithPath: "/x/a.mov"), URL(fileURLWithPath: "/x/b.mov")])
        let (a, b) = (h.model.items[0], h.model.items[1])
        let base = try #require(h.model.settings)
        let catalog = try #require(h.model.catalog)
        let own = catalog.retarget(base, to: try #require(catalog.container(id: "mkv")))
        h.model.override(a.id, with: own)
        h.model.override(b.id, with: base)
        #expect(h.model.items[0].settings == own)
        #expect(h.model.items[1].settings == nil, "settings equal to the window's are no override")

        h.model.convertAll()
        let adds = h.sent.filter { $0.op == "add" }
        #expect(adds.count == 2)
        #expect(adds[0].input == "/x/a.mov" && adds[0].preset == nil && adds[0].presetBody == own)
        #expect(adds[1].input == "/x/b.mov" && adds[1].preset == "apple" && adds[1].presetBody == nil)

        h.model.override(a.id, with: nil)
        #expect(h.model.items[0].settings == own, "a submitted file keeps its settings")
        h.model.handle(try decode(#"{"type":"error","reply":"add:\#(a.id.uuidString)","error":"bad"}"#))
        h.model.retry(a.id)
        #expect(h.model.items[0].settings == own, "retry keeps the file's own settings")
    }

    @Test func inspectorRoutesEditsBySelection() throws {
        let h = Harness()
        try h.load()
        h.model.add(urls: [URL(fileURLWithPath: "/x/a.mov"), URL(fileURLWithPath: "/x/b.mov")])
        let (a, b) = (h.model.items[0].id, h.model.items[1].id)
        let base = try #require(h.model.settings)
        #expect(h.model.inspectorScope == .window)

        h.model.selection = a
        #expect(h.model.inspectorScope == .item(a, name: "a.mov", usesWindow: true))
        var edited = base
        edited.audio.bitrate = 96_000
        h.model.editInspected(edited)
        #expect(h.model.settings == edited, "a file on the window settings edits the window settings")
        #expect(h.model.items[0].settings == nil)

        h.model.setUsesWindowSettings(a, false)
        #expect(h.model.inspectorScope == .item(a, name: "a.mov", usesWindow: false))
        #expect(h.model.items[0].settings == edited, "the override starts as a copy of the window settings")
        #expect(h.model.inspected == edited)
        var own = edited
        own.audio.bitrate = 320_000
        h.model.editInspected(own)
        #expect(h.model.items[0].settings == own, "edits go to the override")
        #expect(h.model.settings == edited, "the window settings are untouched")
        #expect(h.model.items[1].settings == nil)
        #expect(h.model.inspectedOriginal == edited, "Reset on an override goes back to the window settings")
        h.model.editInspected(edited)
        #expect(h.model.inspectorScope == .item(a, name: "a.mov", usesWindow: false),
                "an override equal to the window settings stays an override")

        h.model.selection = b
        #expect(h.model.inspected == edited)
        h.model.convertAll()
        #expect(h.model.inspectorScope == .window, "a submitted file is no longer editable")

        h.model.setUsesWindowSettings(a, true)
        #expect(h.model.items[0].settings == edited, "a submitted file keeps its settings")
    }

    @Test func removingTheSelectionClearsIt() throws {
        let h = Harness()
        try h.load()
        h.model.add(urls: [URL(fileURLWithPath: "/x/a.mov")])
        let a = h.model.items[0].id
        h.model.selection = a
        #expect(h.model.canRemoveSelection)
        h.model.removeSelection()
        #expect(h.model.items.isEmpty)
        #expect(h.model.selection == nil)
        #expect(!h.model.canRemoveSelection)
    }

    @Test func choosingTargets() throws {
        let h = Harness()
        try h.load()
        #expect(h.model.selectedTargetID == "preset:apple")
        h.model.choose(.container("mkv"))
        #expect(h.model.presetID == "")
        #expect(h.model.settings?.container == "mkv")
        #expect(h.model.selectedTargetID == "container:mkv")
        h.model.choose(.container("amr"))
        #expect(h.model.settings?.container == "mkv", "an unavailable format is not chosen")
        h.model.choose(.preset("web"))
        #expect(h.model.selectedTargetID == "preset:web")
        h.model.settings?.audio.bitrate = 64_000
        h.model.choose(.preset("web"))
        #expect(!h.model.isEdited, "choosing the selected preset again drops the edits")
        h.model.choose(.preset("gone"))
        #expect(h.model.presetID == "web")

        h.model.handle(Message(type: "presets", reply: "presets", presets: h.model.presets, recent: ["web", "apple"]))
        #expect(h.model.recent == ["web", "apple"])
        #expect(h.model.targetSections()[0].targets.map(\.id) == ["recent:web", "recent:apple"])
        #expect(h.model.targetSections(search: "zzz").allSatisfy { $0.targets.isEmpty })
    }

    @Test func inlinePresetEncodesBackendKeys() throws {
        let h = Harness()
        try h.load()
        let body = try #require(h.model.settings)
        let data = try JSONEncoder().encode(Request(id: "1", op: "add", input: "/x/a.mov", presetBody: body))
        let json = try #require(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        let preset = try #require(json["presetBody"] as? [String: Any])
        #expect(preset["container"] as? String == "mp4")
        let video = try #require(preset["video"] as? [String: Any])
        #expect(video["rateControl"] as? String == "quality")
        #expect(video["maxWidth"] as? Int == 3840)
    }

    @Test func catalogAudioRules() throws {
        let h = Harness()
        try h.load()
        let catalog = try #require(h.model.catalog)
        let mp3 = try #require(catalog.codec(id: "mp3"))
        #expect(catalog.maxAudioBitrate(for: mp3, sampleRate: 22050, channels: 2) == 160_000)
        #expect(catalog.maxAudioBitrate(for: mp3, sampleRate: nil, channels: nil) == 320_000)
        #expect(catalog.audioBitrates(for: mp3, sampleRate: nil, channels: nil).last == 320_000)
        let opus = try #require(catalog.codec(id: "opus"))
        #expect(catalog.maxAudioBitrate(for: opus, sampleRate: 48000, channels: 1) == 256_000)
        let flac = try #require(catalog.codec(id: "flac"))
        #expect(catalog.audioBitrates(for: flac, sampleRate: nil, channels: nil).isEmpty)
        let aac = try #require(catalog.codec(id: "aac"))
        let m4a = try #require(catalog.container(id: "m4a"))
        #expect(catalog.sampleRates(for: aac, in: m4a).contains(96000), "the software aac encoder goes past 48 kHz")
        let swf = try #require(catalog.container(id: "swf"))
        let swfAudio = catalog.audioSettings(mp3, in: swf, keeping: Preset.Audio(codec: "", sampleRate: 48000))
        #expect(swfAudio.sampleRate == 44100, "a container's own rate list wins")
        #expect(!catalog.isUsable(try #require(catalog.container(id: "amr"))))
        #expect(Converter.presetID(for: "---", taken: []) == "preset")
        #expect(Converter.presetID(for: "Recent", taken: []) == "recent-custom")
    }

    @Test func splitReply() {
        let id = UUID()
        #expect(Converter.split("add:" + id.uuidString) == ("add", id))
        #expect(Converter.split("presets") == ("presets", nil))
        #expect(Converter.split(nil) == ("", nil))
        #expect(Converter.split("add:not-a-uuid") == ("add", nil))
    }
}

@Suite struct FormatTests {
    @Test(arguments: [
        (Int64(0), "0:00"), (Int64(59_400_000_000), "0:59"), (Int64(83_000_000_000), "1:23"),
        (Int64(3_723_000_000_000), "1:02:03"), (Int64(-5), "0:00"),
    ])
    func duration(ns: Int64, want: String) {
        #expect(Format.duration(ns: ns) == want)
    }

    @Test func saving() {
        #expect(Format.saving(from: 100, to: 38) == "62% smaller")
        #expect(Format.saving(from: 100, to: 150) == "50% larger")
        #expect(Format.saving(from: 1000, to: 1001) == "Same size")
        #expect(Format.saving(from: 0, to: 10) == nil)
    }

    @Test func encoderNames() throws {
        var p = JobProgress(percent: 0, pass: 1, passes: 1, elapsedNs: 0, speed: 0, fps: 0, etaNs: 0,
                            encoder: "hevc_videotoolbox", hardware: true)
        #expect(Format.encoder(p) == "Media Engine")
        p.hardware = false
        p.encoder = "libx265"
        #expect(Format.encoder(p) == "Software HEVC")
        p.encoder = nil
        #expect(Format.encoder(p) == nil)
    }

    @Test func audioSummary() {
        let info = MediaInfo(format: "mp3", durationNs: 61_000_000_000, size: 0,
                             streams: [MediaStream(index: 0, kind: "audio", codec: "mp3", channels: 2)])
        #expect(Format.summary(info) == "MP3 · 1:01")
    }
}

@MainActor
@Suite struct BackendTests {
    @Test func locatePrefersEnvironment() throws {
        let dir = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let bin = dir.appending(path: "icelos")
        try Data("#!/bin/sh\n".utf8).write(to: bin)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: bin.path)
        #expect(Backend.locate(environment: ["ICELOS_BACKEND": bin.path])?.path == bin.path)
    }

    @Test func roundTripWithFakeServer() async throws {
        let dir = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let bin = dir.appending(path: "icelos")
        let script = """
        #!/bin/sh
        echo '{"type":"ready","version":"fake"}'
        while read -r line; do echo '{"type":"ok","reply":"echo"}'; done
        echo "bye" >&2
        """
        try Data(script.utf8).write(to: bin)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: bin.path)

        let backend = Backend(executable: bin)
        var types: [String] = []
        var exitMessage: String?
        backend.onMessage = { types.append($0.type) }
        backend.onExit = { exitMessage = $0 }
        try backend.start()
        backend.send(Request(op: "list"))
        for _ in 0..<200 where types.count < 2 { try await Task.sleep(for: .milliseconds(10)) }
        #expect(types == ["ready", "ok"])
        backend.stop()
        for _ in 0..<200 where exitMessage == nil { try await Task.sleep(for: .milliseconds(10)) }
        #expect(exitMessage == "bye")
        backend.send(Request(op: "list"))
    }
}

@Suite struct LineSplitterTests {
    @Test func splitsAcrossChunks() {
        final class Box: @unchecked Sendable { var lines: [String] = [] }
        let box = Box()
        let s = LineSplitter { box.lines.append(String(decoding: $0, as: UTF8.self)) }
        s.feed(Data("ab".utf8))
        s.feed(Data("c\n\nde\nf".utf8))
        #expect(box.lines == ["abc", "de"])
        s.finish()
        #expect(box.lines == ["abc", "de", "f"])
        s.finish()
        #expect(box.lines.count == 3)
    }
}
