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
        let sections = Targets.sections(presets: presets + [mine], recent: ["web", "gone", "my-phone"],
                                        catalog: catalog)
        #expect(sections.map(\.kind) == [.recent, .presets, .video, .audio, .custom])
        #expect(sections.map(\.title) == ["Recent", "Presets", "Video", "Audio", "Custom"])
        #expect(sections[0].targets.map(\.id) == ["recent:web", "recent:my-phone"], "unknown recent IDs are skipped")
        #expect(sections[1].targets.count == 8)
        #expect(sections[1].targets.first?.pick == .preset("apple"))
        #expect(sections[1].targets.first?.symbol == "apple.logo")
        #expect(sections[2].targets.count == catalog.videoContainers.count)
        #expect(sections[2].targets.allSatisfy { $0.symbol == "film" })
        #expect(sections[3].targets.count == catalog.audioContainers.count)
        #expect(sections[4].targets.map(\.id) == ["preset:my-phone"])
        #expect(sections[4].targets.first?.symbol == "slider.horizontal.3")
        let ids = sections.dropFirst().flatMap(\.targets).map(\.id)
        #expect(Set(ids).count == ids.count, "row IDs are unique outside Recent")
    }

    @Test func recentIsCapped() throws {
        let (catalog, presets) = try fixtures()
        let sections = Targets.sections(presets: presets, recent: presets.map(\.id), catalog: catalog)
        #expect(sections[0].targets.count == Targets.recentLimit)
    }

    @Test func noCatalogLeavesFormatsEmpty() throws {
        let (_, presets) = try fixtures()
        let sections = Targets.sections(presets: presets, recent: [], catalog: nil)
        #expect(sections[1].targets.count == 8)
        #expect(sections[2].targets.isEmpty && sections[3].targets.isEmpty)
    }

    @Test func dimmedEntries() throws {
        let (catalog, presets) = try fixtures()
        let audio = Targets.sections(presets: presets, recent: [], catalog: catalog)[3].targets
        let amr = try #require(audio.first { $0.pick == .container("amr") })
        #expect(!amr.available)
        #expect(amr.detail == Targets.unavailableHelp)
        let mp3 = try #require(audio.first { $0.pick == .container("mp3") })
        #expect(mp3.available)
        #expect(mp3.detail == "MP3 with MP3 audio.")
    }

    @Test(arguments: [
        ("webm", ["container:webm"]),
        ("WEBM", ["container:webm"]),
        ("  apple ", ["preset:apple"]),
        ("hevc", ["preset:apple"]),
    ])
    func searchFilters(query: String, want: [String]) throws {
        let (catalog, presets) = try fixtures()
        let ids = Targets.sections(presets: presets, recent: [], catalog: catalog, search: query)
            .flatMap(\.targets).map(\.id)
        for id in want { #expect(ids.contains(id), "\(query) finds \(id)") }
        #expect(ids.count < presets.count + catalog.containers.count, "\(query) filters something")
    }

    @Test func searchWithNoMatchEmptiesEverySection() throws {
        let (catalog, presets) = try fixtures()
        let sections = Targets.sections(presets: presets, recent: ["apple"], catalog: catalog, search: "zzz")
        #expect(sections.count == 5)
        #expect(sections.allSatisfy { $0.targets.isEmpty })
    }

    @Test func selectedID() {
        #expect(Targets.selectedID(presetID: "apple", container: "mp4") == "preset:apple")
        #expect(Targets.selectedID(presetID: "", container: "mkv") == "container:mkv")
        #expect(Targets.selectedID(presetID: "", container: nil) == nil)
    }
}
