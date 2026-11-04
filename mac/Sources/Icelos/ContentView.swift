import AppKit
import IcelosCore
import SwiftUI

/// ContentView is the window: output choices in the sidebar, files in the middle, and the settings
/// in the inspector.
struct ContentView: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// window holds the view-only state the menu commands also reach.
    @Bindable var window: WindowState
    /// dropTargeted is true while files hover over the window.
    @State private var dropTargeted = false

    var body: some View {
        NavigationSplitView {
            TargetList(model: model, search: window.search)
                .searchable(text: $window.search, isPresented: $window.searching, placement: .sidebar,
                            prompt: "Search")
                .navigationSplitViewColumnWidth(min: 200, ideal: 220, max: 300)
        } detail: {
            VStack(spacing: 0) {
                if let fault = model.fault { FaultBanner(text: fault) }
                Group {
                    if model.items.isEmpty {
                        DropZone(targeted: dropTargeted) { MediaFiles.choose { model.add(urls: $0) } }
                    } else {
                        JobList(model: model, targeted: dropTargeted, editSettings: editOwnSettings)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.top, 12)
                BottomBar(model: model)
            }
            .foregroundStyle(Theme.text)
            .background(Theme.backdrop)
            .dropDestination(for: URL.self) { urls, _ in
                let files = MediaFiles.expand(urls)
                model.add(urls: files)
                return !files.isEmpty
            } isTargeted: { dropTargeted = $0 }
            .inspector(isPresented: $window.showsInspector) {
                Inspector(model: model)
                    .inspectorColumnWidth(min: 280, ideal: 312, max: 420)
            }
            .toolbar { toolbar }
        }
    }

    /// toolbar holds Add Files, the destination, the encoder choice, and the inspector toggle.
    @ToolbarContentBuilder private var toolbar: some ToolbarContent {
        ToolbarItemGroup(placement: .primaryAction) {
            Button {
                MediaFiles.choose { model.add(urls: $0) }
            } label: {
                Label("Add Files", systemImage: "plus")
            }
            .help("Add videos or music to convert.")
            DestinationMenu(model: model)
            EngineToggle(model: model)
            Button {
                window.showsInspector.toggle()
            } label: {
                Label(window.showsInspector ? "Hide Inspector" : "Show Inspector", systemImage: "sidebar.trailing")
            }
            .help(window.showsInspector ? "Hide the settings." : "Show the settings.")
        }
    }

    /// editOwnSettings selects a file, gives it its own copy of the window settings, and shows the
    /// inspector on it.
    private func editOwnSettings(_ id: UUID) {
        model.selection = id
        model.setUsesWindowSettings(id, false)
        window.showsInspector = true
    }
}

/// DestinationMenu picks where converted files go.
struct DestinationMenu: View {
    /// model is the window state.
    @Bindable var model: Converter

    var body: some View {
        Menu {
            Button("Next to Originals") { model.outputFolder = nil }
            Button("Choose Folder…") { MediaFiles.chooseFolder { model.outputFolder = $0 } }
            if let folder = model.outputFolder {
                Divider()
                Button("Show \(folder.lastPathComponent) in Finder") { NSWorkspace.shared.open(folder) }
            }
        } label: {
            Label(model.outputFolder?.lastPathComponent ?? "Next to Originals", systemImage: "folder")
                .labelStyle(.titleAndIcon)
        }
        .help("Where converted files go.")
        .accessibilityLabel("Destination")
        .accessibilityValue(model.outputFolder?.lastPathComponent ?? "Next to originals")
    }
}

/// EngineToggle turns the Apple media engine on and off and shows which encoder will run.
struct EngineToggle: View {
    /// model is the window state.
    @Bindable var model: Converter

    var body: some View {
        Toggle(isOn: $model.hardware) {
            Label(model.hasHardware ? "Media Engine" : "Software",
                  systemImage: model.hasHardware && model.hardware ? "bolt.fill" : "cpu")
                .labelStyle(.titleAndIcon)
        }
        .toggleStyle(.button)
        .disabled(!model.hasHardware || blocked)
        .help(help)
        .accessibilityLabel("Use Media Engine")
    }

    /// blocked is true when the window settings cannot use the media engine.
    private var blocked: Bool {
        model.settings?.needsSize == true || model.settings?.isAudioOnly == true
    }

    /// help explains the current encoder choice.
    private var help: String {
        if model.encoders.isEmpty { return "Waiting for the converter." }
        if !model.hasHardware { return "No media engine encoder in this ffmpeg build. Encoding runs in software." }
        if model.settings?.needsSize == true { return "Size targeting uses two software passes for accuracy." }
        if model.settings?.isAudioOnly == true { return "Audio encodes in software." }
        return model.hardware ? "Encoding on the Apple media engine. Falls back to software if it fails."
            : "Encoding in software. Turn on to use the Apple media engine."
    }
}

/// BottomBar summarizes the run and holds Convert.
struct BottomBar: View {
    /// model is the window state.
    @Bindable var model: Converter

    var body: some View {
        HStack(spacing: 12) {
            summary
            Spacer()
            if model.hasFinished {
                Button("Clear Finished") { model.clearFinished() }
                    .buttonStyle(.bordered)
                    .help("Remove finished files from the list.")
            }
            Button {
                model.convertAll()
            } label: {
                Text(model.pendingCount > 1 ? "Convert \(model.pendingCount) Files" : "Convert")
                    .frame(minWidth: 96)
            }
            .buttonStyle(.borderedProminent)
            .disabled(model.pendingCount == 0 || model.settings == nil)
            .keyboardShortcut(AppCommand.convert.shortcut)
        }
        .controlSize(.large)
        .padding(.horizontal, 20)
        .padding(.vertical, 14)
    }

    /// summary shows overall progress while busy, then the space saved, else what is ready.
    @ViewBuilder private var summary: some View {
        if let progress = model.overallProgress {
            HStack(spacing: 10) {
                GradientBar(value: progress).frame(width: 140)
                Text("\(Int(progress * 100))%").monospacedDigit().foregroundStyle(Theme.muted)
            }
        } else if model.savedBytes > 0 {
            Label("Saved \(Format.bytes(model.savedBytes))", systemImage: "leaf.fill")
                .foregroundStyle(Theme.success)
        } else if model.pendingCount > 0 {
            Text(model.pendingCount == 1 ? "1 file ready" : "\(model.pendingCount) files ready")
                .foregroundStyle(Theme.muted)
        }
    }
}

/// FaultBanner shows a problem with the converter process.
struct FaultBanner: View {
    /// text is the problem description.
    let text: String

    var body: some View {
        Label(text, systemImage: "exclamationmark.triangle.fill")
            .font(.callout)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(12)
            .background(RoundedRectangle(cornerRadius: 8).fill(Theme.danger.opacity(0.18)))
            .foregroundStyle(Theme.danger)
            .padding(.horizontal, 20)
            .padding(.top, 12)
            .textSelection(.enabled)
    }
}

/// DropZone is the empty state that invites files.
struct DropZone: View {
    /// targeted is true while files hover.
    let targeted: Bool
    /// choose opens the file picker.
    let choose: () -> Void
    /// reduceMotion turns off the hover animation.
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        VStack(spacing: 12) {
            Image(nsImage: NSApplication.shared.applicationIconImage)
                .resizable()
                .frame(width: 96, height: 96)
                .accessibilityHidden(true)
            Text("Drop videos or music here")
                .font(.title3.weight(.medium))
            Text("Form is never final.")
                .foregroundStyle(Theme.muted)
            Button("Choose Files…", action: choose)
                .buttonStyle(.bordered)
                .controlSize(.large)
                .padding(.top, 4)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background {
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(style: StrokeStyle(lineWidth: 1.5, dash: [7, 6]))
                .foregroundStyle(targeted ? Theme.accent : Theme.border)
                .background(RoundedRectangle(cornerRadius: 10).fill(Theme.accent.opacity(targeted ? 0.08 : 0)))
        }
        .animation(Motion.animation(.easeOut(duration: 0.2), reduceMotion: reduceMotion), value: targeted)
    }
}
