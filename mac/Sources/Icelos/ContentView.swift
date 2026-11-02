import IcelosCore
import SwiftUI

/// ContentView is the single window: presets on top, files in the middle, actions below.
struct ContentView: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// dropTargeted is true while files hover over the window.
    @State private var dropTargeted = false

    var body: some View {
        VStack(spacing: 0) {
            header
            if let fault = model.fault { FaultBanner(text: fault) }
            PresetPicker(model: model)
                .padding(.horizontal, 24)
                .padding(.top, 6)
            OptionsBar(model: model)
                .padding(.horizontal, 24)
                .padding(.vertical, 12)
            Group {
                if model.items.isEmpty {
                    DropZone(targeted: dropTargeted) { MediaFiles.choose { model.add(urls: $0) } }
                } else {
                    JobList(model: model, targeted: dropTargeted)
                }
            }
            .padding(.horizontal, 24)
            footer
        }
        .foregroundStyle(Theme.bone)
        .background(Theme.background)
        .dropDestination(for: URL.self) { urls, _ in
            let files = MediaFiles.expand(urls)
            model.add(urls: files)
            return !files.isEmpty
        } isTargeted: { dropTargeted = $0 }
        .preferredColorScheme(.dark)
    }

    /// header shows the wordmark and the encoder status.
    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("icelos")
                .font(.system(size: 30, weight: .semibold, design: .rounded))
                .foregroundStyle(Theme.brand)
            Text("Free video converter")
                .font(.callout)
                .foregroundStyle(Theme.bone.opacity(0.55))
            Spacer()
            if !model.encoders.isEmpty {
                Label(model.hasHardware ? "Media Engine ready" : "Software encoding",
                      systemImage: model.hasHardware ? "bolt.fill" : "cpu")
                    .font(.caption.weight(.medium))
                    .padding(.horizontal, 10)
                    .padding(.vertical, 5)
                    .background(Capsule().fill(Theme.surface()))
                    .foregroundStyle(model.hasHardware ? Theme.emerald : Theme.ice)
            }
        }
        .padding(.horizontal, 24)
        .padding(.top, 28)
        .padding(.bottom, 8)
    }

    /// footer summarizes the run and holds the main actions.
    private var footer: some View {
        HStack(spacing: 12) {
            summary
            Spacer()
            Button("Add Files…") { MediaFiles.choose { model.add(urls: $0) } }
                .buttonStyle(.bordered)
            if model.hasFinished {
                Button("Clear Finished") { model.clearFinished() }
                    .buttonStyle(.bordered)
            }
            Button {
                model.convertAll()
            } label: {
                Text(model.pendingCount > 1 ? "Convert \(model.pendingCount) Files" : "Convert")
                    .frame(minWidth: 96)
            }
            .buttonStyle(.borderedProminent)
            .tint(Theme.emerald)
            .disabled(model.pendingCount == 0 || model.selectedPreset == nil)
            .keyboardShortcut(.return, modifiers: .command)
        }
        .controlSize(.large)
        .padding(.horizontal, 24)
        .padding(.vertical, 16)
        .background(Theme.midnight.opacity(0.6))
    }

    /// summary shows overall progress while busy, then the space saved.
    @ViewBuilder private var summary: some View {
        if let progress = model.overallProgress {
            HStack(spacing: 10) {
                GradientBar(value: progress).frame(width: 140)
                Text("\(Int(progress * 100))%").monospacedDigit().foregroundStyle(Theme.bone.opacity(0.7))
            }
        } else if model.savedBytes > 0 {
            Label("Saved \(Format.bytes(model.savedBytes))", systemImage: "leaf.fill")
                .foregroundStyle(Theme.emerald)
        } else if let version = model.version {
            Text("icelos \(version) · Free and open source · No account, no upsells")
                .font(.caption)
                .foregroundStyle(Theme.bone.opacity(0.4))
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
            .background(RoundedRectangle(cornerRadius: 10).fill(Theme.ember.opacity(0.18)))
            .foregroundStyle(Theme.ember)
            .padding(.horizontal, 24)
            .textSelection(.enabled)
    }
}

/// OptionsBar holds the settings that apply to the next Convert.
struct OptionsBar: View {
    /// model is the window state.
    @Bindable var model: Converter

    var body: some View {
        HStack(spacing: 18) {
            if model.selectedPreset?.needsSize == true {
                HStack(spacing: 6) {
                    Text("Fit under")
                    TextField("25MB", text: $model.targetSize)
                        .textFieldStyle(.roundedBorder)
                        .frame(width: 90)
                }
            }
            if model.selectedPreset?.isAudioOnly == false {
                Toggle("Use Media Engine", isOn: $model.hardware)
                    .disabled(!model.hasHardware || model.selectedPreset?.needsSize == true)
                    .help(model.selectedPreset?.needsSize == true
                          ? "Size targeting uses two software passes for accuracy."
                          : "Encode on the Apple media engine. Falls back to software if it fails.")
            }
            Spacer()
            Menu {
                Button("Next to Originals") { model.outputFolder = nil }
                Button("Choose Folder…") { MediaFiles.chooseFolder { model.outputFolder = $0 } }
            } label: {
                Label(model.outputFolder?.lastPathComponent ?? "Next to originals", systemImage: "folder")
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        }
        .font(.callout)
    }
}

/// DropZone is the empty state that invites files.
struct DropZone: View {
    /// targeted is true while files hover.
    let targeted: Bool
    /// choose opens the file picker.
    let choose: () -> Void

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: "square.and.arrow.down.on.square")
                .font(.system(size: 44, weight: .light))
                .foregroundStyle(Theme.brand)
                .symbolEffect(.bounce, value: targeted)
            Text("Drop videos or music here")
                .font(.title3.weight(.medium))
            Text("Folders work too. Nothing leaves your Mac.")
                .foregroundStyle(Theme.bone.opacity(0.55))
            Button("Choose Files…", action: choose)
                .buttonStyle(.bordered)
                .controlSize(.large)
                .padding(.top, 4)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background {
            RoundedRectangle(cornerRadius: 18)
                .strokeBorder(style: StrokeStyle(lineWidth: 1.5, dash: [7, 6]))
                .foregroundStyle(targeted ? Theme.ice : Theme.bone.opacity(0.18))
                .background(RoundedRectangle(cornerRadius: 18).fill(targeted ? Theme.ice.opacity(0.08) : .clear))
        }
        .animation(.easeOut(duration: 0.2), value: targeted)
    }
}
