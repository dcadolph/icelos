import AppKit
import IcelosCore
import SwiftUI

/// JobList shows every file and its state in a system list, which draws the selection, moves it
/// with the arrow keys, and names the rows to VoiceOver.
struct JobList: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// editSettings gives one file its own settings in the inspector.
    let editSettings: (UUID) -> Void

    var body: some View {
        List(model.items, selection: $model.selection) { item in
            JobRow(item: item, model: model, editSettings: editSettings)
        }
        .listStyle(.inset)
        .onDeleteCommand { model.removeSelection() }
        .onExitCommand { model.selection = nil }
    }
}

/// JobRow draws one file from its row model: thumbnail, name, source, target, and state.
struct JobRow: View {
    /// item is the file shown.
    let item: Item
    /// model receives actions.
    let model: Converter
    /// editSettings gives one file its own settings in the inspector.
    let editSettings: (UUID) -> Void
    /// hovering is true while the pointer is over the row.
    @State private var hovering = false
    /// reduceMotion turns off the row animations.
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        let row = RowModel(item: item, windowSettings: model.settings, catalog: model.catalog)
        HStack(spacing: 14) {
            Thumbnail(image: model.thumbnails[item.url], media: row.media, tone: row.badge.tone)
            VStack(alignment: .leading, spacing: 4) {
                Text(row.name)
                    .font(.body.weight(.medium))
                    .lineLimit(1)
                    .truncationMode(.middle)
                Text(row.source)
                    .font(.callout)
                    .foregroundStyle(ThemeColor(slot: row.sourceTone))
                    .lineLimit(1)
                    .truncationMode(.tail)
                target(row)
            }
            Spacer(minLength: 12)
            BadgeView(badge: row.badge)
            ForEach(row.actions, id: \.self) { action in
                let shown = action == .cancel || hovering || model.selection == item.id
                iconButton(action) { perform(action) }
                    .opacity(shown ? 1 : 0)
                    .allowsHitTesting(shown)
            }
        }
        .padding(.vertical, 6)
        .onHover { hovering = $0 }
        .contextMenu { menu }
        .animation(Motion.animation(.easeOut(duration: 0.2), reduceMotion: reduceMotion), value: item.phase)
    }

    /// target is the line under the source: chips and estimate before converting, the bar and
    /// progress while running, the result or failure after.
    @ViewBuilder private func target(_ row: RowModel) -> some View {
        if let progress = row.progress {
            VStack(alignment: .leading, spacing: 4) {
                ProgressView(value: progress)
                    .progressViewStyle(.linear)
                    .controlSize(.small)
                    .tint(Theme.accent)
                detail(row)
            }
        } else if row.detail != nil {
            detail(row).textSelection(.enabled)
        } else {
            HStack(spacing: 6) {
                if row.ownSettings { Chip(text: "Own settings") }
                ForEach(row.chips, id: \.self) { Chip(text: $0) }
                if row.lossless {
                    Chip(text: "LOSSLESS", tone: .success, tracking: 0.6)
                        .help("Every stream is copied as it is. Nothing is re-encoded.")
                }
                if let estimate = row.estimate {
                    Text(estimate)
                        .font(.caption)
                        .monospacedDigit()
                        .foregroundStyle(Theme.muted)
                        .lineLimit(1)
                        .layoutPriority(-1)
                }
            }
        }
    }

    /// detail draws the progress, result, or failure line.
    private func detail(_ row: RowModel) -> some View {
        Text(row.detail ?? "")
            .font(.caption)
            .monospacedDigit()
            .foregroundStyle(ThemeColor(slot: row.detailTone))
            .lineLimit(2)
    }

    /// perform runs a trailing action.
    private func perform(_ action: RowModel.Action) {
        switch action {
        case .cancel: model.cancel(item.id)
        case .reveal: reveal()
        case .retry: model.retry(item.id)
        case .remove: model.remove(item.id)
        }
    }

    /// iconButton is a borderless symbol button.
    private func iconButton(_ action: RowModel.Action, run: @escaping () -> Void) -> some View {
        Button(action: run) {
            Image(systemName: action.symbol).font(.system(size: 18))
        }
        .buttonStyle(.borderless)
        .foregroundStyle(Theme.muted)
        .help(action.label)
        .accessibilityLabel(action.label)
    }

    /// menu is the row context menu.
    @ViewBuilder private var menu: some View {
        Button("Show Original in Finder") { NSWorkspace.shared.activateFileViewerSelecting([item.url]) }
        if item.phase == .pending && item.probeError == nil {
            if item.settings == nil {
                Button("Use Own Settings") { editSettings(item.id) }
            } else {
                Button("Use Window Settings") { model.setUsesWindowSettings(item.id, true) }
            }
        }
        if item.phase == .done { Button("Show Output in Finder") { reveal() } }
        if item.phase == .queued || item.phase == .running { Button("Cancel") { model.cancel(item.id) } }
        if !(item.phase == .queued || item.phase == .running) { Button("Remove") { model.remove(item.id) } }
    }

    /// reveal selects the output in Finder.
    private func reveal() {
        if let url = item.outputURL { NSWorkspace.shared.activateFileViewerSelecting([url]) }
    }
}

/// Thumbnail is the 96 by 54 tile at the start of a row: a frame of the video, a waveform for
/// sound, with the state color as a dot on its corner.
struct Thumbnail: View {
    /// image is the cached frame, if made.
    let image: CGImage?
    /// media picks the tile drawn without a frame.
    let media: RowModel.Media
    /// tone is the state color of the dot.
    let tone: Palette.Slot

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: 6).fill(Theme.surface)
            if let image {
                Image(decorative: image, scale: 2)
                    .resizable()
                    .aspectRatio(contentMode: .fill)
            } else {
                Image(systemName: media == .audio ? "waveform" : "film")
                    .font(.system(size: 18, weight: .medium))
                    .foregroundStyle(media == .audio ? Theme.accent : Theme.muted)
            }
        }
        .frame(width: 96, height: 54)
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .overlay(alignment: .bottomTrailing) {
            Circle()
                .fill(ThemeColor(slot: tone))
                .frame(width: 6, height: 6)
                .overlay(Circle().strokeBorder(Theme.background, lineWidth: 1))
                .padding(4)
        }
        .accessibilityHidden(true)
    }
}

/// BadgeView is the state label at the end of a row.
struct BadgeView: View {
    /// badge is the label and its color.
    let badge: RowModel.Badge

    var body: some View {
        Text(badge.text)
            .font(.caption.weight(.medium))
            .monospacedDigit()
            .foregroundStyle(ThemeColor(slot: badge.tone))
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .background(RoundedRectangle(cornerRadius: 6).fill(ThemeColor(slot: badge.tone).opacity(0.14)))
            .fixedSize()
    }
}

/// Chip is one short fact about the output, e.g. "MP4" or "1920×1080", outlined in the separator
/// color like a badge in Music.
struct Chip: View {
    /// text is the label.
    let text: String
    /// tone is the label color.
    var tone: Palette.Slot = .muted
    /// tracking spaces the letters, for uppercase labels.
    var tracking: CGFloat = 0

    var body: some View {
        Text(text)
            .font(.caption.weight(.medium))
            .tracking(tracking)
            .monospacedDigit()
            .foregroundStyle(ThemeColor(slot: tone))
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .lineLimit(1)
            .overlay(RoundedRectangle(cornerRadius: 5).strokeBorder(Theme.border, lineWidth: 1))
    }
}
