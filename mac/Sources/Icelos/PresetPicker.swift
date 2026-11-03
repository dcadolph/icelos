import IcelosCore
import SwiftUI

/// PickerTab is one page of the output picker.
enum PickerTab: String, CaseIterable, Identifiable {
    /// presets holds the built-in presets.
    case presets
    /// video lists the video formats from the catalog.
    case video
    /// audio lists the audio formats from the catalog.
    case audio
    /// custom holds the presets the user saved.
    case custom

    /// id identifies the tab.
    var id: String { rawValue }

    /// title is the tab label.
    var title: String {
        switch self {
        case .presets: "Presets"
        case .video: "Video"
        case .audio: "Audio"
        case .custom: "Custom"
        }
    }
}

/// PresetPicker shows the output choices on tabs: built-in presets, catalog formats, and custom
/// presets. The gear on the selected card opens the settings.
struct PresetPicker: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// tab is the page shown.
    @Binding var tab: PickerTab
    /// openSettings opens the settings sheet for the window settings.
    let openSettings: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 12) {
                Picker("Output", selection: $tab) {
                    ForEach(PickerTab.allCases) { Text($0.title).tag($0) }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .fixedSize()
                .help("Presets are ready-made targets. Video and Audio list every format. "
                      + "Custom holds the settings you saved.")
                Spacer()
                if let settings = model.settings {
                    Text(selectionLabel(settings))
                        .font(.caption)
                        .foregroundStyle(Theme.bone.opacity(0.55))
                        .lineLimit(1)
                }
            }
            page
        }
        .animation(.easeOut(duration: 0.15), value: model.settings)
    }

    /// page is the content of the current tab.
    @ViewBuilder private var page: some View {
        switch tab {
        case .presets:
            cards(model.builtinPresets)
        case .video:
            if let catalog = model.catalog {
                formats(catalog.videoContainers, catalog: catalog)
                resolutions(catalog: catalog)
            } else {
                loading
            }
        case .audio:
            if let catalog = model.catalog { formats(catalog.audioContainers, catalog: catalog) } else { loading }
        case .custom:
            if model.customPresets.isEmpty {
                Text("Open the settings with the gear on any card, then choose Save as Preset.")
                    .font(.callout)
                    .foregroundStyle(Theme.bone.opacity(0.55))
                    .frame(maxWidth: .infinity, minHeight: 60)
            } else {
                cards(model.customPresets)
            }
            if let error = model.presetError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .font(.caption)
                    .foregroundStyle(Theme.ember)
            }
        }
    }

    /// loading stands in while the catalog has not arrived.
    private var loading: some View {
        Text("Loading formats…")
            .font(.callout)
            .foregroundStyle(Theme.bone.opacity(0.55))
            .frame(maxWidth: .infinity, minHeight: 60)
    }

    /// cards lays out preset cards.
    private func cards(_ presets: [Preset]) -> some View {
        BalancedGrid(minCellWidth: 160, spacing: 10) {
            ForEach(presets) { preset in
                let selected = preset.id == model.presetID
                PresetCard(preset: preset, selected: selected, edited: selected && model.isEdited,
                           select: { model.presetID = preset.id }, openSettings: openSettings)
                    .contextMenu {
                        if preset.isCustom {
                            Button("Delete Preset", role: .destructive) { model.deletePreset(preset.id) }
                        }
                    }
            }
        }
    }

    /// formats lays out catalog containers as chips.
    private func formats(_ containers: [Container], catalog: Catalog) -> some View {
        BalancedGrid(minCellWidth: 96, spacing: 8) {
            ForEach(containers) { c in
                let usable = catalog.isUsable(c)
                FormatChip(container: c, selected: isSelected(c), usable: usable,
                           summary: catalog.summary(catalog.target(for: c)),
                           select: { if usable { model.select(container: c) } }, openSettings: openSettings)
            }
        }
    }

    /// resolutions shows the resolution row for the selected video format.
    @ViewBuilder private func resolutions(catalog: Catalog) -> some View {
        if model.presetID.isEmpty, let settings = model.settings, let c = model.container, c.kind == "video" {
            if let codec = catalog.codec(id: settings.video.codec),
               !catalog.isFixed(codec, profile: settings.video.profile, in: c) {
                let current = catalog.resolution(of: settings)?.id
                HStack(spacing: 8) {
                    Text(c.name)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(Theme.bone.opacity(0.55))
                    ForEach(catalog.domains.resolutions) { r in
                        Chip(text: r.name, selected: r.id == current)
                            .onTapGesture { model.select(container: c, resolution: r) }
                            .help(r.width == 0 ? "Keep the source frame size."
                                  : "Fit inside \(r.width)×\(r.height). Smaller sources are never upscaled.")
                    }
                }
            } else if settings.video.codec != copyCodec {
                Text("\(c.name) takes fixed frame sizes. Pick one in the settings.")
                    .font(.caption)
                    .foregroundStyle(Theme.bone.opacity(0.55))
            }
        }
    }

    /// isSelected is true when the window settings come straight from container c.
    private func isSelected(_ c: Container) -> Bool {
        model.presetID.isEmpty && model.settings?.container == c.id
    }

    /// selectionLabel describes the window settings next to the tabs.
    private func selectionLabel(_ p: Preset) -> String {
        let summary = model.catalog?.summary(p) ?? p.ext.uppercased()
        if model.presetID.isEmpty { return summary }
        return model.isEdited ? "\(p.name), edited: \(summary)" : "\(p.name): \(summary)"
    }
}

/// PresetCard is one preset tile.
struct PresetCard: View {
    /// preset is the preset shown.
    let preset: Preset
    /// selected is true for the chosen preset.
    let selected: Bool
    /// edited is true when the window settings differ from the preset.
    let edited: Bool
    /// select chooses the preset.
    let select: () -> Void
    /// openSettings opens the settings sheet.
    let openSettings: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Image(systemName: preset.isCustom ? "slider.horizontal.3" : PresetCard.symbol(for: preset.id))
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(selected ? Theme.ice : Theme.bone.opacity(0.7))
                Spacer()
                Text(preset.ext.uppercased())
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(Theme.bone.opacity(0.45))
                if selected { GearButton(action: openSettings) }
            }
            Text(edited ? "\(preset.name) (edited)" : preset.name)
                .font(.callout.weight(.semibold))
                .lineLimit(1)
            Text(preset.description)
                .font(.caption)
                .foregroundStyle(Theme.bone.opacity(0.55))
                .lineLimit(2, reservesSpace: true)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Theme.surface(selected: selected)))
        .overlay(RoundedRectangle(cornerRadius: 12)
            .strokeBorder(selected ? Theme.ice.opacity(0.7) : Color.white.opacity(0.06), lineWidth: 1))
        .contentShape(RoundedRectangle(cornerRadius: 12))
        .onTapGesture(perform: select)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
        .accessibilityAction(.default, select)
        .accessibilityAction(named: "Settings") {
            select()
            openSettings()
        }
        .help(preset.description)
    }

    /// symbol picks an SF Symbol for a preset ID.
    static func symbol(for id: String) -> String {
        switch id {
        case "apple": "apple.logo"
        case "compatible": "play.tv"
        case "web": "globe"
        case "tiny": "paperplane"
        case "target-size": "scalemass"
        case "remux": "shippingbox"
        case "audio": "waveform"
        case "mp3": "music.note"
        default: "film"
        }
    }
}

/// FormatChip is one catalog container tile.
struct FormatChip: View {
    /// container is the format shown.
    let container: Container
    /// selected is true for the chosen format.
    let selected: Bool
    /// usable is false when no codec for the format is available.
    let usable: Bool
    /// summary describes what the format starts with.
    let summary: String
    /// select chooses the format.
    let select: () -> Void
    /// openSettings opens the settings sheet.
    let openSettings: () -> Void

    var body: some View {
        HStack(spacing: 4) {
            Text(container.name)
                .font(.callout.weight(.semibold))
                .lineLimit(1)
                .minimumScaleFactor(0.8)
            Spacer(minLength: 0)
            if selected { GearButton(action: openSettings) }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Theme.surface(selected: selected)))
        .overlay(RoundedRectangle(cornerRadius: 9)
            .strokeBorder(selected ? Theme.ice.opacity(0.7) : Color.white.opacity(0.06), lineWidth: 1))
        .opacity(usable ? 1 : 0.4)
        .contentShape(RoundedRectangle(cornerRadius: 9))
        .onTapGesture(perform: select)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
        .accessibilityAction(.default, select)
        .accessibilityAction(named: "Settings") {
            select()
            if usable { openSettings() }
        }
        .help(usable ? "\(summary) Saves as .\(container.ext)."
              : "\(container.name) needs the bundled ffmpeg, which this build does not have yet.")
    }
}

/// Chip is a small selectable label.
struct Chip: View {
    /// text is the label.
    let text: String
    /// selected is true for the chosen value.
    let selected: Bool

    var body: some View {
        Text(text)
            .font(.caption.weight(.medium))
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(Capsule().fill(Theme.surface(selected: selected)))
            .overlay(Capsule().strokeBorder(selected ? Theme.ice.opacity(0.7) : .clear, lineWidth: 1))
            .contentShape(Capsule())
            .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
    }
}

/// GearButton opens the settings for the card it sits on.
struct GearButton: View {
    /// action opens the settings.
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: "gearshape.fill").font(.system(size: 13))
        }
        .buttonStyle(.borderless)
        .foregroundStyle(Theme.ice)
        .help("Settings")
        .accessibilityLabel("Settings")
    }
}
