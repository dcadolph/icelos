import AppKit
import Foundation
import SwiftUI
import Testing

@testable import IcelosCore

/// sourcesDir is the package's Sources directory, found from this file's path.
private let sourcesDir = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    .appendingPathComponent("Sources")

/// SourceFile is one Swift source file read for the rubric scans.
private struct SourceFile {
    /// name is the file name, e.g. "JobList.swift".
    let name: String
    /// target is the module directory, "Icelos" or "IcelosCore".
    let target: String
    /// lines are the file's lines.
    let lines: [String]
}

/// sources reads every Swift file under Sources, or only the files of one target.
private func sources(_ target: String? = nil) throws -> [SourceFile] {
    let targets = try target.map { [$0] }
        ?? FileManager.default.contentsOfDirectory(atPath: sourcesDir.path).sorted()
    return try targets.flatMap { t in
        let dir = sourcesDir.appendingPathComponent(t)
        return try FileManager.default.contentsOfDirectory(atPath: dir.path).sorted()
            .filter { $0.hasSuffix(".swift") }
            .map { name in
                let text = try String(contentsOf: dir.appendingPathComponent(name), encoding: .utf8)
                return SourceFile(name: name, target: t, lines: text.components(separatedBy: "\n"))
            }
    }
}

/// hits lists "file:line" for every line containing needle.
private func hits(_ needle: String, in files: [SourceFile]) -> [String] {
    files.flatMap { f in
        f.lines.indices.filter { f.lines[$0].contains(needle) }.map { "\(f.name):\($0 + 1)" }
    }
}

/// rubricCatalog decodes the captured catalog reply.
private func rubricCatalog() throws -> Catalog {
    try #require(try JSONDecoder().decode(Message.self, from: Data(catalogFixture.utf8)).catalog)
}

@Suite struct RubricTests {
    // Rubric 1: no literal color outside Palette.swift.
    @Test func colorsLiveOnlyInPalette() throws {
        let files = try sources()
        #expect(!files.isEmpty)
        let found = hits("Color(red", in: files)
        #expect(!found.isEmpty)
        #expect(found.allSatisfy { $0.hasPrefix("Palette.swift:") }, "\(found)")
    }

    // Rubric 3: appearance never forces a color scheme.
    @Test func noForcedColorScheme() throws {
        #expect(hits(".preferredColorScheme", in: try sources()).isEmpty)
    }

    // Rubric 11: settings are never modal.
    @Test func noSheets() throws {
        #expect(hits(".sheet", in: try sources("Icelos")).isEmpty)
    }

    // Rubric 12: the six commands carry the designed shortcuts.
    @Test func shortcuts() {
        let got = Dictionary(uniqueKeysWithValues: AppCommand.allCases.map { ($0, $0.display) })
        let want: [AppCommand: String] = [
            .addFiles: "⌘O", .convert: "⌘↩", .clearFinished: "⌘K", .toggleInspector: "⌥⌘I",
            .searchFormats: "⌘F", .removeSelection: "⌘⌫",
        ]
        #expect(got == want)
        #expect(Set(want.values).count == want.count)
    }

    // Rubric 12: every view shortcut comes from AppCommand, every command is in the menus, and the
    // file list takes the bare Delete key.
    @Test func menusUseTheCommandTable() throws {
        let files = try sources("Icelos")
        let shortcuts = hits(".keyboardShortcut(", in: files)
        let fromTable = hits(".keyboardShortcut(AppCommand.", in: files)
        #expect(shortcuts == fromTable, "\(Set(shortcuts).subtracting(fromTable))")
        let app = try #require(files.first { $0.name == "IcelosApp.swift" }).lines.joined(separator: "\n")
        for command in AppCommand.allCases {
            #expect(app.contains("AppCommand.\(command.rawValue).shortcut"), "\(command) missing from the menus")
        }
        #expect(!hits(".onDeleteCommand", in: files).isEmpty)
    }

    // Rubric 13: every bare symbol image is labeled, hidden, or folded into a labeled element
    // within its view.
    @Test func symbolImagesAreNamed() throws {
        let marks = ["accessibilityLabel", "accessibilityHidden", "accessibilityElement(children:"]
        var unnamed: [String] = []
        for f in try sources("Icelos") {
            for i in f.lines.indices where f.lines[i].contains("Image(systemName:") {
                let window = f.lines[i..<min(i + 15, f.lines.count)].joined(separator: "\n")
                if !marks.contains(where: window.contains) { unnamed.append("\(f.name):\(i + 1)") }
            }
        }
        #expect(unnamed.isEmpty, "\(unnamed)")
    }

    // Rubric 14: Reduce Motion turns animations off.
    @Test func reduceMotionDropsAnimation() {
        #expect(Motion.animation(.easeOut(duration: 0.2), reduceMotion: true) == nil)
        #expect(Motion.animation(.easeOut(duration: 0.2), reduceMotion: false) == .easeOut(duration: 0.2))
    }

    // Rubric 14: every view animation goes through Motion.
    @Test func animationsReadReduceMotion() throws {
        let files = try sources("Icelos")
        let all = hits(".animation(", in: files)
        let gated = hits(".animation(Motion.animation(", in: files)
        #expect(!all.isEmpty)
        #expect(all == gated, "\(Set(all).subtracting(gated))")
        #expect(hits("withAnimation", in: files).isEmpty)
    }

    // Rubric 16: no text explains the UI to itself.
    @Test func noSelfExplainingText() throws {
        let files = try sources()
        #expect(hits("Loading formats", in: files).isEmpty)
        #expect(hits("Open the settings with the gear", in: files).isEmpty)
    }

    // Rubric 22: every sidebar symbol and every symbol literal in Sources exists on this macOS.
    @Test func symbolsResolve() throws {
        let presets = try #require(try JSONDecoder().decode(Message.self, from: Data(presetsFixture.utf8)).presets)
        let sidebar = Targets.allSymbols(presets: presets, catalog: try rubricCatalog())
        let pattern = /(?:systemImage|systemName): "([^"]+)"/
        let literals = try sources().flatMap { f in
            f.lines.flatMap { line in line.matches(of: pattern).map { String($0.output.1) } }
        }
        #expect(!literals.isEmpty)
        let missing = Set(sidebar + literals).filter {
            NSImage(systemSymbolName: $0, accessibilityDescription: nil) == nil
        }
        #expect(missing.isEmpty, "\(missing.sorted())")
    }

    // Rubric 23: a sidebar row is one symbol and one name, with no caption and no painted color.
    @Test func sidebarRowsAreNameAndSymbol() throws {
        let fields = Mirror(reflecting: Target(id: "", pick: .preset(""), name: "", symbol: "", detail: "",
                                               available: true)).children.compactMap(\.label)
        #expect(fields == ["id", "pick", "name", "symbol", "detail", "available"])
        let list = try #require(try sources("Icelos").first { $0.name == "TargetList.swift" })
        let captions = list.lines.indices.filter { list.lines[$0].contains(/\bext\b/) }
        #expect(captions.isEmpty, "TargetList.swift lines \(captions.map { $0 + 1 })")
        #expect(hits("Theme.", in: [list]).isEmpty)
    }

    // Rubric 19: the inspector reaches every catalog domain and every control.
    @Test func inspectorExposesEveryDomain() throws {
        let catalog = try rubricCatalog()
        let fields = Set(Mirror(reflecting: catalog.domains).children.compactMap(\.label))
        #expect(fields == Set(DomainKey.allCases.map(\.rawValue)))

        var drafts: [Preset] = []
        for c in catalog.containers {
            let base = catalog.target(for: c)
            drafts.append(base)
            var exact = base
            exact.video.width = 1920
            exact.video.height = 1080
            drafts.append(exact)
            for vc in catalog.videoCodecs(in: c) {
                let p = catalog.settingVideoCodec(base, to: vc.id)
                drafts.append(p)
                for rc in vc.rateControls ?? [] {
                    var q = p
                    q.video = catalog.withRateControl(p.video, rc, keeping: p.video)
                    drafts.append(q)
                }
            }
            for ac in catalog.audioCodecs(in: c) {
                drafts.append(catalog.settingAudioCodec(base, to: ac.id))
            }
        }
        drafts += try builtins().values
        let controls = Set(drafts.flatMap { catalog.inspectorControls(for: $0) })
        #expect(controls == Set(InspectorControl.allCases), "\(Set(InspectorControl.allCases).subtracting(controls))")
        let reached = Set(controls.flatMap(\.domains))
        #expect(reached == Set(DomainKey.allCases), "\(Set(DomainKey.allCases).subtracting(reached))")
    }

    // Rubric 19: the controls follow the codec rules.
    @Test func inspectorFollowsTheCodec() throws {
        let catalog = try rubricCatalog()
        let presets = try builtins()
        let apple = catalog.inspectorControls(for: try #require(presets["apple"]))
        #expect(apple.contains(.quality) && !apple.contains(.videoBitrate) && !apple.contains(.fitUnder))
        let size = catalog.inspectorControls(for: try #require(presets["target-size"]))
        #expect(size.contains(.fitUnder) && !size.contains(.videoBitrate))
        let mp3 = catalog.inspectorControls(for: try #require(presets["mp3"]))
        #expect(!mp3.contains(.videoEncoder) && !mp3.contains(.includeAudio) && mp3.contains(.audioBitrate))
    }
}
