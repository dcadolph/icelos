import Foundation

/// AppSettings reads and writes the user's preferences in a UserDefaults store.
public struct AppSettings {
    /// Key names the UserDefaults keys, shared with the views that use @AppStorage.
    public enum Key {
        /// theme is the stored ThemeName raw value.
        public static let theme = "theme"
        /// appearance is the stored Appearance raw value.
        public static let appearance = "appearance"
        /// outputFolder is the default destination path, empty for next to originals.
        public static let outputFolder = "outputFolder"
        /// useMediaEngine is true when conversions prefer the media engine.
        public static let useMediaEngine = "useMediaEngine"
    }

    /// Snapshot is a Codable copy of every setting.
    public struct Snapshot: Codable, Equatable, Sendable {
        /// theme is the chosen theme.
        public var theme: ThemeName
        /// appearance is the chosen light or dark mode.
        public var appearance: Appearance
        /// outputFolder is the default destination path, or nil for next to originals.
        public var outputFolder: String?
        /// useMediaEngine is true when conversions prefer the media engine.
        public var useMediaEngine: Bool

        /// Creates a snapshot from its values.
        public init(theme: ThemeName, appearance: Appearance, outputFolder: String?, useMediaEngine: Bool) {
            self.theme = theme
            self.appearance = appearance
            self.outputFolder = outputFolder
            self.useMediaEngine = useMediaEngine
        }
    }

    /// defaults is the backing store.
    private let defaults: UserDefaults

    /// Creates settings over the given store, the standard one by default.
    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    /// theme is the chosen theme, or the default when nothing valid is stored.
    public var theme: ThemeName {
        get { ThemeName(stored: defaults.string(forKey: Key.theme)) }
        nonmutating set { defaults.set(newValue.rawValue, forKey: Key.theme) }
    }

    /// appearance is the chosen light or dark mode, or System when nothing valid is stored.
    public var appearance: Appearance {
        get { defaults.string(forKey: Key.appearance).flatMap(Appearance.init(rawValue:)) ?? .system }
        nonmutating set { defaults.set(newValue.rawValue, forKey: Key.appearance) }
    }

    /// outputFolder is the default destination, or nil for next to each original.
    public var outputFolder: URL? {
        get {
            guard let path = defaults.string(forKey: Key.outputFolder), !path.isEmpty else { return nil }
            return URL(fileURLWithPath: path, isDirectory: true)
        }
        nonmutating set { defaults.set(newValue?.path ?? "", forKey: Key.outputFolder) }
    }

    /// useMediaEngine is true when conversions prefer the media engine, true by default.
    public var useMediaEngine: Bool {
        get { defaults.object(forKey: Key.useMediaEngine) as? Bool ?? true }
        nonmutating set { defaults.set(newValue, forKey: Key.useMediaEngine) }
    }

    /// snapshot returns every setting as one value.
    public var snapshot: Snapshot {
        get {
            Snapshot(theme: theme, appearance: appearance, outputFolder: outputFolder?.path,
                     useMediaEngine: useMediaEngine)
        }
        nonmutating set {
            theme = newValue.theme
            appearance = newValue.appearance
            outputFolder = newValue.outputFolder.flatMap { $0.isEmpty ? nil : URL(fileURLWithPath: $0) }
            useMediaEngine = newValue.useMediaEngine
        }
    }
}
