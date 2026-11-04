import AppKit
import IcelosCore
import SwiftUI

/// SettingsView is the Settings window content with the General and Appearance tabs, each a
/// grouped form in the system style.
struct SettingsView: View {
    /// theme is the stored theme choice.
    @AppStorage(AppSettings.Key.theme) private var theme = ThemeName.standard
    /// appearance is the stored light or dark choice.
    @AppStorage(AppSettings.Key.appearance) private var appearance = Appearance.system
    /// outputFolder is the stored destination path, empty for next to originals.
    @AppStorage(AppSettings.Key.outputFolder) private var outputFolder = ""
    /// useMediaEngine is the stored media engine default.
    @AppStorage(AppSettings.Key.useMediaEngine) private var useMediaEngine = true
    /// threads is the stored encoder thread cap, 0 for automatic.
    @AppStorage(AppSettings.Key.threads) private var threads = 0
    /// colorScheme picks which palette of a two-appearance theme the dots show.
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        TabView {
            general.tabItem { Label("General", systemImage: "gearshape") }
            looks.tabItem { Label("Appearance", systemImage: "paintpalette") }
        }
    }

    /// general is the destination, media engine, and CPU limit tab.
    private var general: some View {
        Form {
            Picker("Default destination", selection: Binding(
                get: { outputFolder.isEmpty },
                set: { nextToOriginals in
                    if nextToOriginals { outputFolder = "" } else { chooseFolder() }
                })) {
                Text("Next to originals").tag(true)
                Text(folderLabel).tag(false)
            }
            .pickerStyle(.radioGroup)
            if !outputFolder.isEmpty {
                Button("Choose Folder…") { chooseFolder() }
            }
            Toggle("Use Media Engine by default", isOn: $useMediaEngine)
            Stepper(value: $threads, in: 0...ProcessInfo.processInfo.activeProcessorCount) {
                Text(threads == 0 ? "CPU limit: Automatic"
                    : "CPU limit: \(threads) of \(ProcessInfo.processInfo.activeProcessorCount) cores")
                    .monospacedDigit()
            }
            .help("Caps the threads a software encode uses, so the Mac stays responsive. Automatic lets ffmpeg decide.")
        }
        .formStyle(.grouped)
        .frame(width: 480)
        .fixedSize(horizontal: false, vertical: true)
    }

    /// looks is the appearance and theme tab.
    private var looks: some View {
        Form {
            Picker("Appearance", selection: $appearance) {
                ForEach(Appearance.allCases, id: \.self) { Text($0.name).tag($0) }
            }
            .pickerStyle(.segmented)
            LabeledContent("Theme") {
                VStack(alignment: .trailing, spacing: 6) {
                    HStack(spacing: 6) {
                        ForEach(ThemeName.allCases, id: \.self) { dot($0) }
                    }
                    Text(theme.name)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .formStyle(.grouped)
        .frame(width: 480)
        .fixedSize(horizontal: false, vertical: true)
    }

    /// dot is one theme's swatch: its accent ringed in its background, checked when chosen, the
    /// shape of the accent color row in System Settings.
    private func dot(_ item: ThemeName) -> some View {
        let palette = item.palette(dark: colorScheme == .dark)
        let chosen = item == theme
        return Button { theme = item } label: {
            Circle()
                .fill(palette.accent.color)
                .frame(width: 20, height: 20)
                .overlay(Circle().strokeBorder(palette.background.color, lineWidth: 1))
                .overlay {
                    if chosen {
                        Image(systemName: "checkmark")
                            .font(.system(size: 9, weight: .bold))
                            .foregroundStyle(palette.onAccent.color)
                            .accessibilityHidden(true)
                    }
                }
        }
        .buttonStyle(.plain)
        .help(item.name)
        .accessibilityLabel(item.name)
        .accessibilityAddTraits(chosen ? .isSelected : [])
    }

    /// folderLabel names the chosen folder, or asks for one.
    private var folderLabel: String {
        outputFolder.isEmpty ? "Folder…" : URL(fileURLWithPath: outputFolder).lastPathComponent
    }

    /// chooseFolder asks for a destination folder and stores it.
    private func chooseFolder() {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.canCreateDirectories = true
        panel.prompt = "Choose"
        if panel.runModal() == .OK, let url = panel.url { outputFolder = url.path }
    }
}
