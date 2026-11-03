import AppKit
import IcelosCore
import SwiftUI

/// JobList shows every file and its state.
struct JobList: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// targeted is true while files hover.
    let targeted: Bool
    /// editSettings opens the settings sheet for one file.
    let editSettings: (UUID) -> Void

    var body: some View {
        ScrollView {
            LazyVStack(spacing: 8) {
                ForEach(model.items) { item in
                    JobRow(item: item, model: model, editSettings: editSettings)
                }
            }
            .padding(.vertical, 4)
        }
        .scrollIndicators(.automatic)
        .overlay {
            RoundedRectangle(cornerRadius: 14)
                .strokeBorder(Theme.ice.opacity(targeted ? 0.7 : 0), lineWidth: 1.5)
                .allowsHitTesting(false)
        }
    }
}

/// JobRow is one file with its progress and actions.
struct JobRow: View {
    /// item is the file shown.
    let item: Item
    /// model receives actions.
    let model: Converter
    /// editSettings opens the settings sheet for one file.
    let editSettings: (UUID) -> Void

    var body: some View {
        HStack(spacing: 14) {
            icon
            VStack(alignment: .leading, spacing: 5) {
                Text(item.url.lastPathComponent)
                    .font(.callout.weight(.medium))
                    .lineLimit(1)
                    .truncationMode(.middle)
                detail
            }
            Spacer(minLength: 12)
            actions
        }
        .padding(12)
        .background(RoundedRectangle(cornerRadius: 12).fill(Theme.surface()))
        .contextMenu { menu }
        .animation(.easeOut(duration: 0.2), value: item.phase)
    }

    /// icon shows the media kind and the outcome.
    private var icon: some View {
        let audio = item.info.map { $0.video == nil } ?? false
        return ZStack {
            RoundedRectangle(cornerRadius: 9).fill(Theme.deep)
            Image(systemName: iconName(audio: audio))
                .font(.system(size: 16, weight: .medium))
                .foregroundStyle(iconColor)
        }
        .frame(width: 38, height: 38)
    }

    /// iconName picks the row symbol.
    private func iconName(audio: Bool) -> String {
        switch item.phase {
        case .done: "checkmark"
        case .failed: "exclamationmark"
        case .canceled: "xmark"
        default: audio ? "waveform" : "film"
        }
    }

    /// iconColor tints the row symbol.
    private var iconColor: Color {
        switch item.phase {
        case .done: Theme.emerald
        case .failed: Theme.ember
        case .canceled: Theme.bone.opacity(0.5)
        default: Theme.ice
        }
    }

    /// detail is the line under the file name.
    @ViewBuilder private var detail: some View {
        switch item.phase {
        case .pending:
            secondary(item.probeError ?? pendingText, color: item.probeError == nil ? nil : Theme.ember)
        case .queued:
            secondary("Waiting…")
        case .running:
            VStack(alignment: .leading, spacing: 5) {
                GradientBar(value: item.status?.progress.percent ?? 0)
                secondary(runningText)
            }
        case .done:
            secondary(doneText, color: Theme.emerald)
        case .failed:
            secondary(item.errorMessage ?? "Failed.", color: Theme.ember)
                .textSelection(.enabled)
        case .canceled:
            secondary("Canceled.")
        }
    }

    /// pendingText describes a file waiting for Convert, with its own settings when it has them.
    private var pendingText: String {
        let summary = item.info.map(Format.summary) ?? "Reading…"
        guard let s = item.settings else { return summary }
        return "\(summary) · Own settings: \(s.ext.uppercased())"
    }

    /// runningText describes live progress.
    private var runningText: String {
        guard let p = item.status?.progress else { return "Starting…" }
        var parts = ["\(Int(p.percent * 100))%"]
        if p.passes > 1 { parts.append("pass \(p.pass) of \(p.passes)") }
        if p.speed > 0 { parts.append(String(format: "%.1f×", p.speed)) }
        if p.etaNs > 0 { parts.append(Format.duration(ns: p.etaNs) + " left") }
        if let enc = Format.encoder(p) { parts.append(enc) }
        return parts.joined(separator: " · ")
    }

    /// doneText describes the finished output.
    private var doneText: String {
        var parts = [item.outputURL?.lastPathComponent ?? "Done"]
        if let out = item.outputSize {
            parts.append(Format.bytes(out))
            if let src = item.info?.size, let saving = Format.saving(from: src, to: out) { parts.append(saving) }
        }
        return parts.joined(separator: " · ")
    }

    /// secondary styles a detail line.
    private func secondary(_ text: String, color: Color? = nil) -> some View {
        Text(text)
            .font(.caption)
            .monospacedDigit()
            .foregroundStyle(color ?? Theme.bone.opacity(0.55))
            .lineLimit(2)
    }

    /// actions are the trailing buttons for the current phase.
    @ViewBuilder private var actions: some View {
        switch item.phase {
        case .queued, .running:
            iconButton("stop.circle", help: "Cancel") { model.cancel(item.id) }
        case .done:
            iconButton("magnifyingglass.circle", help: "Show in Finder") { reveal() }
            iconButton("xmark.circle", help: "Remove from list") { model.remove(item.id) }
        case .failed, .canceled:
            if item.probeError == nil {
                iconButton("arrow.clockwise.circle", help: "Try again") { model.retry(item.id) }
            }
            iconButton("xmark.circle", help: "Remove from list") { model.remove(item.id) }
        case .pending:
            iconButton("xmark.circle", help: "Remove from list") { model.remove(item.id) }
        }
    }

    /// iconButton is a borderless symbol button.
    private func iconButton(_ symbol: String, help: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: symbol).font(.system(size: 18))
        }
        .buttonStyle(.borderless)
        .foregroundStyle(Theme.bone.opacity(0.7))
        .help(help)
        .accessibilityLabel(help)
    }

    /// menu is the row context menu.
    @ViewBuilder private var menu: some View {
        Button("Show Original in Finder") { NSWorkspace.shared.activateFileViewerSelecting([item.url]) }
        if item.phase == .pending && item.probeError == nil {
            Button("Settings for This File…") { editSettings(item.id) }
            if item.settings != nil { Button("Use Window Settings") { model.override(item.id, with: nil) } }
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
