import Foundation
import Testing

@testable import IcelosCore

/// fixtures decodes the captured catalog and presets replies.
private func fixtures() throws -> (Catalog, [Preset]) {
    let catalog = try #require(try JSONDecoder().decode(Message.self, from: Data(catalogFixture.utf8)).catalog)
    let presets = try #require(try JSONDecoder().decode(Message.self, from: Data(presetsFixture.utf8)).presets)
    return (catalog, presets)
}

/// custom returns a saved preset built from p.
private func custom(_ p: Preset, id: String, name: String) -> Preset {
    var c = p
    c.id = id
    c.name = name
    c.custom = true
    return c
}

@Suite struct TargetsTests {
    @Test func sectionsInOrder() throws {
        let (catalog, presets) = try fixtures()
        let mine = custom(presets[0], id: "my-phone", name: "My Phone")
        let sections = Targets.sections(presets: presets + [mine], catalog: catalog)
        #expect(sections.map(\.kind) == [.presets, .video, .audio, .custom])
        #expect(sections.map(\.title) == ["Presets", "Video", "Audio", "Custom"])
        #expect(sections[0].targets.map(\.name) == [
            "Apple Devices", "Plays Anywhere", "Web and Sharing", "Tiny File", "Fit a Size",
            "Change Container", "Audio Only", "MP3",
        ])
        #expect(sections[0].targets.first?.pick == .preset("apple"))
        #expect(sections[0].targets.first?.symbol == "apple.logo")
        #expect(sections[0].more.isEmpty && sections[3].more.isEmpty)
        #expect(sections[1].targets.map(\.pick) == catalog.commonVideoContainers.map { .container($0.id) })
        #expect(sections[1].more.map(\.pick) == catalog.moreVideoContainers.map { .container($0.id) })
        #expect(sections[2].targets.map(\.pick) == catalog.commonAudioContainers.map { .container($0.id) })
        #expect(sections[2].more.map(\.pick) == catalog.moreAudioContainers.map { .container($0.id) })
        #expect(sections[3].targets.map(\.id) == ["preset:my-phone"])
        #expect(sections[3].targets.first?.symbol == "slider.horizontal.3")
        let ids = sections.flatMap { $0.targets + $0.more }.map(\.id)
        #expect(Set(ids).count == ids.count, "row IDs are unique")
    }

    @Test func formatsSplitIntoCommonAndMore() throws {
        let (catalog, presets) = try fixtures()
        let sections = Targets.sections(presets: presets, catalog: catalog)
        #expect(sections[1].targets.count == 12 && sections[1].more.count == 10)
        #expect(sections[2].targets.count == 11 && sections[2].more.count == 7)
        #expect(sections[1].targets.map(\.name) == [
            "MP4", "MOV", "MKV", "M4V", "AVI", "WMV", "WebM", "MPG", "TS", "M2TS", "3GP", "MXF",
        ])
        #expect(sections[2].more.map(\.name) == ["AC3", "WMA", "MKA", "CAF", "AU", "AMR", "MP2"])
    }

    @Test func noCatalogLeavesFormatsEmpty() throws {
        let (_, presets) = try fixtures()
        let sections = Targets.sections(presets: presets, catalog: nil)
        #expect(sections[0].targets.count == 8)
        #expect(sections[1].isEmpty && sections[2].isEmpty)
    }

    @Test func dimmedEntries() throws {
        let (catalog, presets) = try fixtures()
        let audio = Targets.sections(presets: presets, catalog: catalog)[2]
        let amr = try #require(audio.more.first { $0.pick == .container("amr") })
        #expect(!amr.available)
        #expect(amr.detail == Targets.unavailableHelp)
        let mp3 = try #require(audio.targets.first { $0.pick == .container("mp3") })
        #expect(mp3.available)
        #expect(mp3.detail == "MP3 with MP3 audio.")
    }

    @Test(arguments: [
        ("webm", ["container:webm"]),
        ("WEBM", ["container:webm"]),
        ("  apple ", ["preset:apple"]),
        ("hevc", ["preset:apple"]),
        ("swf", ["container:swf"]),
    ])
    func searchFilters(query: String, want: [String]) throws {
        let (catalog, presets) = try fixtures()
        let ids = Targets.sections(presets: presets, catalog: catalog, search: query)
            .flatMap { $0.targets + $0.more }.map(\.id)
        for id in want { #expect(ids.contains(id), "\(query) finds \(id)") }
        #expect(ids.count < presets.count + catalog.containers.count, "\(query) filters something")
    }

    @Test func searchReachesMoreFormats() throws {
        let (catalog, presets) = try fixtures()
        let video = Targets.sections(presets: presets, catalog: catalog, search: "swf")[1]
        #expect(video.targets.isEmpty)
        #expect(video.more.map(\.id) == ["container:swf"])
    }

    @Test func searchWithNoMatchEmptiesEverySection() throws {
        let (catalog, presets) = try fixtures()
        let sections = Targets.sections(presets: presets, catalog: catalog, search: "zzz")
        #expect(sections.count == 4)
        #expect(sections.allSatisfy { $0.targets.isEmpty && $0.more.isEmpty })
    }

    @Test func selectedID() {
        #expect(Targets.selectedID(presetID: "apple", container: "mp4") == "preset:apple")
        #expect(Targets.selectedID(presetID: "", container: "mkv") == "container:mkv")
        #expect(Targets.selectedID(presetID: "", container: nil) == nil)
    }

    @Test func containerSymbolsByFamily() throws {
        let (catalog, _) = try fixtures()
        let want: [String: String] = [
            "mp4": "film", "mkv": "film.stack", "webm": "globe", "wmv": "pc", "vob": "opticaldisc",
            "trp": "antenna.radiowaves.left.and.right", "3g2": "phone", "amr": "phone",
            "mxf": "movieclapper", "dv": "video", "swf": "play.rectangle", "opus": "music.note",
            "alac": "waveform", "m4r": "bell", "m4b": "book.closed", "ac3": "hifispeaker",
        ]
        for (id, symbol) in want {
            let c = try #require(catalog.container(id: id), "\(id) is in the catalog")
            #expect(Targets.symbol(forContainer: c.id, kind: c.kind) == symbol, "\(id)")
        }
        #expect(Targets.symbol(forContainer: "new-video", kind: "video") == "film")
        #expect(Targets.symbol(forContainer: "new-audio", kind: "audio") == "waveform")
        let mapped = Set(catalog.containers.map { Targets.symbol(forContainer: $0.id, kind: $0.kind) })
        #expect(mapped.count == 15, "every family has a member in the catalog")
    }

    @Test func allSymbolsCoverEveryRow() throws {
        let (catalog, presets) = try fixtures()
        let all = Set(Targets.allSymbols(presets: presets, catalog: catalog))
        let drawn = Targets.sections(presets: presets, catalog: catalog)
            .flatMap { $0.targets + $0.more }.map(\.symbol)
        #expect(Set(drawn).isSubset(of: all))
        #expect(all.contains("slider.horizontal.3"))
    }
}
