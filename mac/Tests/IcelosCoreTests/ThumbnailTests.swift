import AVFoundation
import CoreGraphics
import CoreVideo
import Foundation
import Testing

@testable import IcelosCore

/// writeClip writes a silent H.264 clip of seconds at 10 fps and 320 by 180 to url.
private func writeClip(_ url: URL, seconds: Int) async throws {
    let writer = try AVAssetWriter(outputURL: url, fileType: .mov)
    let input = AVAssetWriterInput(mediaType: .video, outputSettings: [
        AVVideoCodecKey: AVVideoCodecType.h264, AVVideoWidthKey: 320, AVVideoHeightKey: 180,
    ])
    let adaptor = AVAssetWriterInputPixelBufferAdaptor(assetWriterInput: input, sourcePixelBufferAttributes: [
        kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA,
        kCVPixelBufferWidthKey as String: 320, kCVPixelBufferHeightKey as String: 180,
    ])
    writer.add(input)
    #expect(writer.startWriting())
    writer.startSession(atSourceTime: .zero)
    for frame in 0..<(seconds * 10) {
        while !input.isReadyForMoreMediaData { try await Task.sleep(nanoseconds: 1_000_000) }
        var buffer: CVPixelBuffer?
        CVPixelBufferPoolCreatePixelBuffer(nil, try #require(adaptor.pixelBufferPool), &buffer)
        let pixels = try #require(buffer)
        CVPixelBufferLockBaseAddress(pixels, [])
        let base = try #require(CVPixelBufferGetBaseAddress(pixels))
        memset(base, Int32(frame * 2 % 256), CVPixelBufferGetDataSize(pixels))
        CVPixelBufferUnlockBaseAddress(pixels, [])
        adaptor.append(pixels, withPresentationTime: CMTime(value: CMTimeValue(frame), timescale: 10))
    }
    input.markAsFinished()
    await writer.finishWriting()
    #expect(writer.status == .completed)
}

/// tinyImage returns a one-pixel image.
private func tinyImage() -> CGImage {
    let ctx = CGContext(data: nil, width: 1, height: 1, bitsPerComponent: 8, bytesPerRow: 4,
                        space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    return ctx.makeImage()!
}

/// probeLine is a probe reply for id with one stream of kind.
private func probeLine(_ id: UUID, kind: String) -> String {
    let size = kind == "video" ? #","width":320,"height":180"# : ""
    return #"{"type":"probe","reply":"probe:\#(id.uuidString)","info":{"format":"mov","durationNs":10000000000,"size":100,"streams":[{"index":0,"codec_type":"\#(kind)","codec_name":"h264"\#(size)}]}}"#
}

/// Asked counts thumbnail requests across tasks.
private actor Asked {
    /// urls lists every source asked for, in order.
    var urls: [URL] = []

    /// add records one request.
    func add(_ url: URL) { urls.append(url) }
}

@Suite struct ThumbnailTests {
    @Test func tenSecondClipYieldsAFrameUnderASecond() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let clip = dir.appendingPathComponent("clip.mov")
        try await writeClip(clip, seconds: 10)

        let start = ContinuousClock.now
        let image = try #require(await Thumbnails.image(for: clip))
        let took = ContinuousClock.now - start
        #expect(took < .seconds(1), "took \(took)")
        #expect(image.width <= Int(Thumbnails.size.width) && image.height <= Int(Thumbnails.size.height))
        #expect(image.width == 192 && image.height == 108)
    }

    @Test func unreadableFileHasNoFrame() async {
        #expect(await Thumbnails.image(for: URL(fileURLWithPath: "/nonexistent/clip.mov")) == nil)
    }

    @MainActor @Test func converterAsksOncePerVideoSource() async throws {
        let asked = Asked()
        let image = tinyImage()
        let model = Converter(send: { _ in }, fileSize: { _ in nil }, thumbnail: { url in
            await asked.add(url)
            return image
        })
        let clip = URL(fileURLWithPath: "/x/clip.mov"), song = URL(fileURLWithPath: "/x/song.m4a")
        model.add(urls: [clip, song])
        let (a, b) = (model.items[0].id, model.items[1].id)
        model.handle(try JSONDecoder().decode(Message.self, from: Data(probeLine(a, kind: "video").utf8)))
        model.handle(try JSONDecoder().decode(Message.self, from: Data(probeLine(a, kind: "video").utf8)))
        model.handle(try JSONDecoder().decode(Message.self, from: Data(probeLine(b, kind: "audio").utf8)))
        await model.thumbnailTasks[clip]?.value
        #expect(await asked.urls == [clip])
        #expect(model.thumbnails[clip] != nil)
        #expect(model.thumbnails[song] == nil)

        model.remove(a)
        #expect(model.thumbnails[clip] == nil)
        #expect(model.thumbnailTasks[clip] == nil)
    }
}

@Suite struct ThumbnailStressTests {
    @MainActor @Test func defaultThumbnailerOnManyConverters() async throws {
        for _ in 0..<30 {
            let model = Converter(send: { _ in })
            model.add(urls: [URL(fileURLWithPath: "/x/clip.mov")])
            let id = model.items[0].id
            model.handle(try JSONDecoder().decode(Message.self, from: Data(probeLine(id, kind: "video").utf8)))
            await model.thumbnailTasks[URL(fileURLWithPath: "/x/clip.mov")]?.value
        }
    }
}
