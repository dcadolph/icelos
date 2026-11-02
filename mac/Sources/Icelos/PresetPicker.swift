import IcelosCore
import SwiftUI

/// PresetPicker shows the presets as selectable cards.
struct PresetPicker: View {
    /// model is the window state.
    @Bindable var model: Converter

    var body: some View {
        LazyVGrid(columns: [GridItem(.adaptive(minimum: 148), spacing: 10)], spacing: 10) {
            ForEach(model.presets) { preset in
                PresetCard(preset: preset, selected: preset.id == model.presetID)
                    .onTapGesture { model.presetID = preset.id }
            }
        }
        .animation(.easeOut(duration: 0.15), value: model.presetID)
    }
}

/// PresetCard is one preset tile.
struct PresetCard: View {
    /// preset is the preset shown.
    let preset: Preset
    /// selected is true for the chosen preset.
    let selected: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Image(systemName: PresetCard.symbol(for: preset.id))
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(selected ? Theme.ice : Theme.bone.opacity(0.7))
                Spacer()
                Text(preset.ext.uppercased())
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(Theme.bone.opacity(0.45))
            }
            Text(preset.name)
                .font(.callout.weight(.semibold))
                .lineLimit(1)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(Theme.surface(selected: selected)))
        .overlay(RoundedRectangle(cornerRadius: 12)
            .strokeBorder(selected ? Theme.ice.opacity(0.7) : Color.white.opacity(0.06), lineWidth: 1))
        .contentShape(RoundedRectangle(cornerRadius: 12))
        .help(preset.description)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
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
