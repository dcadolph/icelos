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
}

@MainActor
@Suite struct ConverterTests {
    @Test func readyAsksForPresetsAndCapabilities() throws {
        let h = Harness()
        h.model.handle(try decode(#"{"type":"ready","version":"v1.0.0"}"#))
        #expect(h.model.version == "v1.0.0")
        #expect(h.sent.map(\.op) == ["presets", "capabilities"])
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
