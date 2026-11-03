import IcelosCore
import SwiftUI

/// TargetList is the sidebar of output choices: recent and built-in presets, every catalog format,
/// and the presets the user saved.
struct TargetList: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// search filters every section.
    let search: String

    var body: some View {
        let sections = model.targetSections(search: search).filter { !$0.targets.isEmpty }
        List(selection: Binding(get: { model.selectedTargetID }, set: { pick($0, in: sections) })) {
            ForEach(sections) { section in
                Section(section.title) {
                    ForEach(section.targets) { target in
                        TargetRow(target: target)
                            .tag(target.id)
                            .selectionDisabled(!target.available)
                            .contextMenu {
                                if section.kind == .custom, case .preset(let id) = target.pick {
                                    Button("Delete Preset", role: .destructive) { model.deletePreset(id) }
                                }
                            }
                    }
                }
            }
        }
        .listStyle(.sidebar)
        .overlay {
            if sections.isEmpty && !search.isEmpty {
                Text("No matches")
                    .font(.callout)
                    .foregroundStyle(Theme.muted)
            }
        }
    }

    /// pick chooses the row with id.
    private func pick(_ id: String?, in sections: [TargetSection]) {
        guard let id, let target = sections.lazy.flatMap(\.targets).first(where: { $0.id == id }),
              target.available else { return }
        model.choose(target.pick)
    }
}

/// TargetRow is one output choice: symbol, name, and the extension it writes.
struct TargetRow: View {
    /// target is the choice shown.
    let target: Target

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: target.symbol)
                .frame(width: 18)
                .foregroundStyle(Theme.accent)
            Text(target.name)
                .lineLimit(1)
            Spacer(minLength: 6)
            Text(target.ext.uppercased())
                .font(.caption.weight(.medium))
                .tracking(0.6)
                .foregroundStyle(Theme.muted)
        }
        .opacity(target.available ? 1 : 0.45)
        .help(target.detail)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(target.available ? "\(target.name), \(target.ext)" : "\(target.name), unavailable")
        .accessibilityHint(target.detail)
    }
}
