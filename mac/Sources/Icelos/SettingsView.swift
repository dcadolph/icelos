import AppKit
import IcelosCore
import SwiftUI

/// SettingsView is the Settings window content with the Look and Output tabs.
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
    /// colorScheme is the scheme the swatches preview.
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        TabView {
            look.tabItem { Label("Look", systemImage: "paintpalette") }
            output.tabItem { Label("Output", systemImage: "square.and.arrow.down") }
        }
        .frame(width: 460, height: 420)
        .background(Theme.backdrop)
        .themed()
    }

    /// look is the Appearance and Theme tab.
    private var look: some View {
        VStack(alignment: .leading, spacing: 14) {
            Picker("Appearance", selection: $appearance) {
                ForEach(Appearance.allCases, id: \.self) { Text($0.name).tag($0) }
            }
            .pickerStyle(.segmented)
            .foregroundStyle(Theme.text)
            Text("Theme").font(.headline).foregroundStyle(Theme.text)
            List(ThemeName.allCases, id: \.self) { item in
                Button { theme = item } label: { themeRow(item) }
                    .buttonStyle(.plain)
                    .listRowBackground(Rectangle().fill(Theme.fill(selected: item == theme)))
            }
            .scrollContentBackground(.hidden)
        }
        .padding(20)
    }

    /// themeRow shows a theme's name with swatches of its background, accent, and text.
    private func themeRow(_ item: ThemeName) -> some View {
        let palette = item.palette(dark: colorScheme == .dark)
        return HStack(spacing: 8) {
            ForEach([palette.background, palette.accent, palette.text], id: \.self) { swatch in
                RoundedRectangle(cornerRadius: 5)
                    .fill(swatch.color)
                    .frame(width: 24, height: 24)
                    .overlay(RoundedRectangle(cornerRadius: 5).stroke(Theme.border))
            }
            Text(item.name).foregroundStyle(Theme.text)
            Spacer()
            if item == theme { Image(systemName: "checkmark").foregroundStyle(Theme.accentText) }
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }

    /// output is the destination and media engine tab.
    private var output: some View {
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
                Text(threads == 0 ? "CPU limit: Automatic" : "CPU limit: \(threads) of \(ProcessInfo.processInfo.activeProcessorCount) cores")
                    .monospacedDigit()
            }
            .help("Caps the threads a software encode uses, so the Mac stays responsive. Automatic lets ffmpeg decide.")
        }
        .foregroundStyle(Theme.text)
        .padding(20)
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
