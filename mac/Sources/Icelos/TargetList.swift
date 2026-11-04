import IcelosCore
import SwiftUI

/// TargetList is the sidebar of output choices: the built-in presets, the catalog formats with the
/// rare ones folded under More Formats, and the presets the user saved.
struct TargetList: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// search filters every section.
    let search: String
    /// expanded holds the sections whose More Formats group the user opened.
    @State private var expanded: Set<TargetSection.Kind> = []

    var body: some View {
        let sections = model.targetSections(search: search).filter { !$0.isEmpty }
        List(selection: Binding(get: { model.selectedTargetID }, set: { pick($0, in: sections) })) {
            ForEach(sections) { section in
                Section(section.title) {
                    ForEach(section.targets) { target in
                        row(target, in: section)
                    }
                    if !section.more.isEmpty {
                        DisclosureGroup("More Formats", isExpanded: isExpanded(section.kind)) {
                            ForEach(section.more) { target in
                                row(target, in: section)
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
                    .foregroundStyle(.secondary)
            }
        }
        .onAppear { reveal(model.selectedTargetID, in: sections) }
        .onChange(of: model.selectedTargetID) { _, id in reveal(id, in: sections) }
    }

    /// row is one selectable target with its context menu.
    private func row(_ target: Target, in section: TargetSection) -> some View {
        TargetRow(target: target)
            .tag(target.id)
            .selectionDisabled(!target.available)
            .contextMenu {
                if section.kind == .custom, case .preset(let id) = target.pick {
                    Button("Delete Preset", role: .destructive) { model.deletePreset(id) }
                }
            }
    }

    /// isExpanded binds a section's More Formats group, which stays open while a search runs.
    private func isExpanded(_ kind: TargetSection.Kind) -> Binding<Bool> {
        Binding(get: { !search.isEmpty || expanded.contains(kind) },
                set: { open in
                    if open { expanded.insert(kind) } else { expanded.remove(kind) }
                })
    }

    /// reveal opens the More Formats group that holds the row with id.
    private func reveal(_ id: String?, in sections: [TargetSection]) {
        guard let id else { return }
        for section in sections where section.more.contains(where: { $0.id == id }) {
            expanded.insert(section.kind)
        }
    }

    /// pick chooses the row with id.
    private func pick(_ id: String?, in sections: [TargetSection]) {
        guard let id,
              let target = sections.lazy.flatMap({ $0.targets + $0.more }).first(where: { $0.id == id }),
              target.available else { return }
        model.choose(target.pick)
    }
}

/// TargetRow is one output choice: a symbol and a name in the system sidebar style.
struct TargetRow: View {
    /// target is the choice shown.
    let target: Target

    var body: some View {
        if target.available {
            Label(target.name, systemImage: target.symbol)
                .help(target.detail)
                .accessibilityHint(target.detail)
        } else {
            Label(target.name, systemImage: target.symbol)
                .foregroundStyle(.tertiary)
                .help(target.detail)
                .accessibilityLabel("\(target.name), unavailable")
                .accessibilityHint(target.detail)
        }
    }
}
