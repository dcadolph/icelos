import Foundation
import Testing
@testable import IcelosCore

/// withScratchDefaults runs body against a throwaway UserDefaults suite and removes it after.
func withScratchDefaults(_ body: (UserDefaults) throws -> Void) rethrows {
    let name = "icelos-tests-\(UUID())"
    let defaults = UserDefaults(suiteName: name)!
    defer { defaults.removePersistentDomain(forName: name) }
    try body(defaults)
}

@Test func appSettingsDefaults() {
    withScratchDefaults { defaults in
        let settings = AppSettings(defaults: defaults)
        #expect(settings.theme == .standard)
        #expect(settings.appearance == .system)
        #expect(settings.outputFolder == nil)
        #expect(settings.useMediaEngine == true)
    }
}

@Test func appSettingsRoundTrip() throws {
    try withScratchDefaults { defaults in
        let settings = AppSettings(defaults: defaults)
        let snap = AppSettings.Snapshot(theme: .harbor, appearance: .dark, outputFolder: "/tmp/out",
                                        useMediaEngine: false)
        settings.snapshot = snap
        #expect(AppSettings(defaults: defaults).snapshot == snap)
        let decoded = try JSONDecoder().decode(AppSettings.Snapshot.self, from: JSONEncoder().encode(snap))
        #expect(decoded == snap)
        settings.outputFolder = nil
        #expect(settings.outputFolder == nil)
    }
}

@Test func appSettingsUnknownThemeFallsBack() {
    withScratchDefaults { defaults in
        defaults.set("no-such-theme", forKey: AppSettings.Key.theme)
        defaults.set("sepia", forKey: AppSettings.Key.appearance)
        let settings = AppSettings(defaults: defaults)
        #expect(settings.theme == .standard)
        #expect(settings.appearance == .system)
    }
}

@Test func appSettingsKeysMatchViews() {
    #expect(AppSettings.Key.theme == "theme")
    #expect(AppSettings.Key.appearance == "appearance")
}
