import IcelosCore
import SwiftUI

/// SettingsSheet edits one set of conversion settings with controls built from the catalog, so a
/// new codec in the backend needs no change here.
struct SettingsSheet: View {
    /// catalog supplies the containers, codecs, and value lists.
    let catalog: Catalog
    /// title names what is being edited.
    let title: String
    /// original is what Reset goes back to.
    let original: Preset
    /// error is the latest preset save problem.
    let error: String?
    /// apply receives the edited settings on Done.
    let apply: (Preset) -> Void
    /// save stores the edited settings as a custom preset with a name.
    let save: (Preset, String) -> Void

    /// draft is the settings being edited.
    @State private var draft: Preset
    /// presetName is the name typed for Save as Preset.
    @State private var presetName = ""
    /// naming is true while the Save as Preset field shows.
    @State private var naming = false
    /// dismiss closes the sheet.
    @Environment(\.dismiss) private var dismiss

    /// Creates a sheet that starts from current and resets to original.
    init(catalog: Catalog, title: String, current: Preset, original: Preset, error: String?,
         apply: @escaping (Preset) -> Void, save: @escaping (Preset, String) -> Void) {
        self.catalog = catalog
        self.title = title
        self.original = original
        self.error = error
        self.apply = apply
        self.save = save
        _draft = State(initialValue: current)
    }

    var body: some View {
        VStack(spacing: 0) {
            Text(title)
                .font(.headline)
                .padding(.top, 18)
            Form {
                formatSection
                if container?.kind == "video" { videoSection }
                audioSection
            }
            .formStyle(.grouped)
            if let error {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .font(.callout)
                    .foregroundStyle(Theme.ember)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 20)
                    .textSelection(.enabled)
            }
            footer
        }
        .frame(width: 520, height: 640)
    }

    /// container is the draft's catalog container.
    private var container: Container? { catalog.container(id: draft.container) }

    /// videoCodec is the draft's catalog video codec.
    private var videoCodec: Codec? { catalog.codec(id: draft.video.codec) }

    /// audioCodec is the draft's catalog audio codec.
    private var audioCodec: Codec? { catalog.codec(id: draft.audio.codec) }

    /// formatSection picks the container.
    private var formatSection: some View {
        Section {
            Picker("Format", selection: Binding(
                get: { draft.container ?? "" },
                set: { id in if let c = catalog.container(id: id) { draft = catalog.retarget(draft, to: c) } })) {
                Section("Video") {
                    ForEach(catalog.videoContainers) { containerRow($0) }
                }
                Section("Audio") {
                    ForEach(catalog.audioContainers) { containerRow($0) }
                }
            }
            .help("The file type to write. Changing it resets the encoders to ones this format carries.")
        }
    }

    /// containerRow is one Format choice, dimmed when no codec for it is available.
    private func containerRow(_ c: Container) -> some View {
        let usable = catalog.isUsable(c)
        return Text(usable ? "\(c.name) (.\(c.ext))" : "\(c.name) (needs the bundled ffmpeg)")
            .tag(c.id)
            .disabled(!usable)
    }

    /// codecRow is one encoder choice, dimmed when this ffmpeg build cannot encode it.
    private func codecRow(_ c: Codec) -> some View {
        Text(c.available ? c.name : "\(c.name) (needs the bundled ffmpeg)")
            .tag(c.id)
            .disabled(!c.available)
    }

    /// videoSection holds the video controls for the draft's codec.
    @ViewBuilder private var videoSection: some View {
        Section("Video") {
            if let container {
                Picker("Encoder", selection: Binding(
                    get: { draft.video.codec },
                    set: { draft = catalog.settingVideoCodec(draft, to: $0) })) {
                    Text("Copy from source").tag(copyCodec)
                    ForEach(catalog.videoCodecs(in: container)) { codecRow($0) }
                }
                .help("The video codec. Copy from source keeps the original stream with no quality loss.")
                if let codec = videoCodec {
                    if catalog.isFixed(codec, profile: draft.video.profile, in: container) {
                        modePicker(codec, container)
                    } else {
                        resolutionPicker
                    }
                    if !catalog.modes(for: codec, profile: draft.video.profile, in: container)
                        .contains(where: { $0.fps != nil }) {
                        Picker("Frame rate", selection: Binding(
                            get: { draft.video.fps ?? 0 },
                            set: { draft.video.fps = $0 == 0 ? nil : $0 })) {
                            ForEach(catalog.frameRates(for: codec), id: \.fps) { Text($0.name).tag($0.fps) }
                        }
                        .help("Frames per second. Same as source keeps the original timing.")
                    }
                    rateControls(codec)
                    codecOptions(codec, container)
                } else if draft.video.codec == copyCodec {
                    note("The video stream is copied as it is. Nothing else about it changes.")
                }
            }
        }
    }

    /// resolutionPicker offers the resolution ladder as a fit box, or an exact frame size.
    @ViewBuilder private var resolutionPicker: some View {
        let exact = draft.video.width != nil || draft.video.height != nil
        let current = exact ? "custom" : (catalog.resolution(of: draft)?.id ?? "box")
        Picker("Resolution", selection: Binding(
            get: { current },
            set: { id in
                if id == "custom" {
                    draft.video.maxWidth = nil
                    draft.video.maxHeight = nil
                    draft.video.width = draft.video.width ?? 1920
                    draft.video.height = draft.video.height ?? 1080
                } else if let r = catalog.domains.resolutions.first(where: { $0.id == id }) {
                    draft = catalog.fitting(draft, into: r)
                }
            })) {
            ForEach(catalog.domains.resolutions) { r in
                Text(r.width == 0 ? r.name : "\(r.name) (up to \(r.width)×\(r.height))").tag(r.id)
            }
            if current == "box" {
                Text("Up to \(draft.video.maxWidth ?? 0)×\(draft.video.maxHeight ?? 0)").tag("box")
            }
            Text("Custom size").tag("custom")
        }
        .help("Each size is a box the video fits inside, never upscaled. Custom size sets an exact frame.")
        if exact {
            LabeledContent("Frame size") {
                HStack(spacing: 6) {
                    dimensionField($draft.video.width)
                    Text("×")
                    dimensionField($draft.video.height)
                }
            }
            if let problem = sizeProblem {
                note(problem, color: Theme.ember)
            }
        }
    }

    /// sizeProblem explains why the exact frame size would be refused.
    private var sizeProblem: String? {
        let d = catalog.domains
        let sizes = [draft.video.width ?? 0, draft.video.height ?? 0]
        if sizes.contains(where: { $0 % 2 != 0 || $0 < d.minDimension || $0 > d.maxDimension }) {
            return "Width and height must be even numbers from \(d.minDimension) to \(d.maxDimension)."
        }
        return nil
    }

    /// dimensionField is a pixel count entry.
    private func dimensionField(_ value: Binding<Int?>) -> some View {
        TextField("", value: value, format: .number.grouping(.never))
            .textFieldStyle(.roundedBorder)
            .multilineTextAlignment(.trailing)
            .frame(width: 70)
    }

    /// modePicker offers the fixed frame shapes a format takes.
    private func modePicker(_ codec: Codec, _ container: Container) -> some View {
        let modes = catalog.modes(for: codec, profile: draft.video.profile, in: container)
        let index = modes.firstIndex { $0.width == draft.video.width && $0.height == draft.video.height
            && ($0.fps == nil || $0.fps == draft.video.fps) } ?? -1
        return Picker("Frame size", selection: Binding(
            get: { index },
            set: { i in
                guard modes.indices.contains(i) else {
                    draft.video.width = nil
                    draft.video.height = nil
                    if modes.contains(where: { $0.fps != nil }) { draft.video.fps = nil }
                    return
                }
                draft.video.width = modes[i].width
                draft.video.height = modes[i].height
                if let fps = modes[i].fps { draft.video.fps = fps }
            })) {
            Text("Match the source").tag(-1)
            ForEach(modes.indices, id: \.self) { i in
                let m = modes[i]
                Text(m.fps.map { "\(m.name), \(m.width)×\(m.height) at \(Self.rate($0)) fps" }
                     ?? "\(m.name), \(m.width)×\(m.height)").tag(i)
            }
        }
        .help("This format only takes these frame sizes. Match the source picks the closest one.")
    }

    /// rateControls offers the codec's rate controls and the quality or bitrate each needs.
    @ViewBuilder private func rateControls(_ codec: Codec) -> some View {
        let controls = codec.rateControls ?? []
        if !controls.isEmpty, let rc = draft.video.rateControl {
            Picker("Rate control", selection: Binding(
                get: { rc },
                set: { draft.video = catalog.withRateControl(draft.video, $0, keeping: draft.video) })) {
                ForEach(controls, id: \.self) { Text(Self.rateControlName($0)).tag($0) }
            }
            .help("Constant quality lets the size vary. Bitrate modes aim for a data rate. Two-pass is slower and more exact.")
            switch rc {
            case RateControl.quality:
                Picker("Quality", selection: Binding(
                    get: { draft.video.quality ?? QualityLevel.high },
                    set: { draft.video.quality = $0 })) {
                    Text("High").tag(QualityLevel.high)
                    Text("Balanced").tag(QualityLevel.balanced)
                    Text("Small").tag(QualityLevel.small)
                }
                .pickerStyle(.segmented)
                .help("High keeps nearly all detail. Small gives the smallest file.")
            case RateControl.twopass where draft.needsSize:
                note("Two passes aim for the size set under Fit under.")
            default:
                LabeledContent("Bitrate") {
                    BitrateField(bitsPerSecond: $draft.video.bitrate, choices: catalog.domains.videoBitrates)
                }
            }
        }
    }

    /// codecOptions offers the codec's profiles, levels, and tags.
    @ViewBuilder private func codecOptions(_ codec: Codec, _ container: Container) -> some View {
        if let profiles = codec.profiles, !profiles.isEmpty {
            let required = (codec.modes ?? []).contains { $0.profile != nil }
            Picker("Profile", selection: Binding(
                get: { draft.video.profile ?? "" },
                set: { id in
                    let wasFixed = catalog.isFixed(codec, profile: draft.video.profile, in: container)
                    draft.video.profile = id.isEmpty ? nil : id
                    if wasFixed != catalog.isFixed(codec, profile: draft.video.profile, in: container) {
                        draft.video.width = nil
                        draft.video.height = nil
                        draft.video.maxWidth = nil
                        draft.video.maxHeight = nil
                        draft.video.bitrate = nil
                    }
                })) {
                if !required { Text("Automatic").tag("") }
                ForEach(profiles, id: \.id) { Text($0.name).tag($0.id) }
            }
            .help("The codec profile. Older devices may need a simpler one.")
        }
        if let levels = codec.levels, !levels.isEmpty {
            Picker("Level", selection: Binding(
                get: { draft.video.level ?? "" },
                set: { draft.video.level = $0.isEmpty ? nil : $0 })) {
                Text("Automatic").tag("")
                ForEach(levels, id: \.self) { Text($0).tag($0) }
            }
            .help("Caps resolution and data rate for device compatibility. Automatic picks one that fits.")
        }
        if let tags = codec.tags, !tags.isEmpty {
            Picker("Codec tag", selection: Binding(
                get: { draft.video.tag ?? "" },
                set: { draft.video.tag = $0.isEmpty ? nil : $0 })) {
                Text("Default").tag("")
                ForEach(tags, id: \.self) { Text($0).tag($0) }
            }
            .help("The codec tag players look for, such as DIVX or XVID for older hardware.")
        }
    }

    /// audioSection holds the audio controls for the draft's codec.
    @ViewBuilder private var audioSection: some View {
        Section("Audio") {
            if let container {
                if container.kind == "video" {
                    Toggle("Include audio", isOn: Binding(
                        get: { !draft.audio.codec.isEmpty },
                        set: { on in
                            let id = on ? (catalog.defaultCodec("audio", in: container)?.id ?? "") : ""
                            draft = catalog.settingAudioCodec(draft, to: id)
                        }))
                        .help("Turn off to write a silent video.")
                }
                if !draft.audio.codec.isEmpty {
                    Picker("Encoder", selection: Binding(
                        get: { draft.audio.codec },
                        set: { draft = catalog.settingAudioCodec(draft, to: $0) })) {
                        Text("Copy from source").tag(copyCodec)
                        ForEach(catalog.audioCodecs(in: container)) { codecRow($0) }
                    }
                    .help("The audio codec. Copy from source keeps the original audio untouched.")
                    if let codec = audioCodec {
                        audioOptions(codec, container)
                    } else if draft.audio.codec == copyCodec {
                        note("The audio stream is copied as it is. Nothing else about it changes.")
                    }
                }
            }
        }
    }

    /// audioOptions offers sample rate, channels, bitrate, and bit depth for the codec.
    @ViewBuilder private func audioOptions(_ codec: Codec, _ container: Container) -> some View {
        let rates = catalog.sampleRates(for: codec, in: container)
        Picker("Sample rate", selection: Binding(
            get: { draft.audio.sampleRate ?? 0 },
            set: { draft.audio.sampleRate = $0 == 0 ? nil : $0; clampAudioBitrate(codec) })) {
            if container.sampleRates?.isEmpty != false { Text("Same as source").tag(0) }
            ForEach(rates, id: \.self) { Text(Self.hertz($0)).tag($0) }
        }
        .help("Samples per second. Use 48 kHz for video and 44.1 kHz for music.")
        let layouts = catalog.channelLayouts(for: codec, in: container)
        Picker("Channels", selection: Binding(
            get: { draft.audio.channels ?? 0 },
            set: { draft.audio.channels = $0 == 0 ? nil : $0; clampAudioBitrate(codec) })) {
            if container.channels == nil { Text("Same as source").tag(0) }
            ForEach(layouts, id: \.count) { Text($0.name).tag($0.count) }
        }
        .help("Mono, stereo, or surround. Same as source keeps the original layout.")
        let choices = catalog.audioBitrates(for: codec, sampleRate: draft.audio.sampleRate,
                                            channels: draft.audio.channels)
        if let fixed = codec.bitrates, !fixed.isEmpty {
            Picker("Bitrate", selection: Binding(
                get: { draft.audio.bitrate ?? fixed.max() ?? 0 },
                set: { draft.audio.bitrate = $0 })) {
                ForEach(fixed, id: \.self) { Text(BitrateField.label($0)).tag($0) }
            }
        } else if !choices.isEmpty {
            LabeledContent("Bitrate") {
                BitrateField(bitsPerSecond: $draft.audio.bitrate, choices: choices)
            }
            let cap = catalog.maxAudioBitrate(for: codec, sampleRate: draft.audio.sampleRate,
                                              channels: draft.audio.channels)
            if cap > 0, let b = draft.audio.bitrate, b > cap {
                note("\(codec.name) tops out at \(BitrateField.label(cap)) here.", color: Theme.ember)
            }
        }
        if let depths = codec.bitDepths, depths.count > 1 {
            Picker("Bit depth", selection: Binding(
                get: { draft.audio.bitDepth ?? 0 },
                set: { draft.audio.bitDepth = $0 == 0 ? nil : $0 })) {
                Text("Same as source").tag(0)
                ForEach(depths, id: \.self) { Text("\($0)-bit").tag($0) }
            }
            .help("Bits per sample. 24-bit keeps studio detail. 16-bit matches CD audio.")
        }
    }

    /// clampAudioBitrate lowers the audio bitrate to the codec's cap after a rate or channel change.
    private func clampAudioBitrate(_ codec: Codec) {
        let cap = catalog.maxAudioBitrate(for: codec, sampleRate: draft.audio.sampleRate,
                                          channels: draft.audio.channels)
        if cap > 0, let b = draft.audio.bitrate, b > cap {
            draft.audio.bitrate = catalog.audioBitrates(for: codec, sampleRate: draft.audio.sampleRate,
                                                        channels: draft.audio.channels).last ?? cap
        }
    }

    /// footer holds Reset, Save as Preset, Cancel, and Done.
    private var footer: some View {
        HStack(spacing: 10) {
            Button("Reset") { draft = original }
            .help("Go back to the settings this sheet started from.")
                .disabled(draft == original)
            if naming {
                TextField("Preset name", text: $presetName)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 160)
                    .onSubmit(saveDraft)
                Button("Save", action: saveDraft)
                    .disabled(presetName.trimmingCharacters(in: .whitespaces).isEmpty)
            } else {
                Button("Save as Preset…") { naming = true }
                .help("Keep these settings as a preset on the Custom tab.")
            }
            Spacer()
            Button("Cancel", role: .cancel) { dismiss() }
                .keyboardShortcut(.cancelAction)
            Button("Done") {
                apply(draft)
                dismiss()
            }
            .keyboardShortcut(.defaultAction)
            .disabled(sizeProblem != nil && (draft.video.width != nil || draft.video.height != nil))
        }
        .padding(16)
    }

    /// saveDraft sends the draft to be saved as a custom preset.
    private func saveDraft() {
        let name = presetName.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { return }
        save(draft, name)
        naming = false
        presetName = ""
    }

    /// note is a secondary line of explanation.
    private func note(_ text: String, color: Color? = nil) -> some View {
        Text(text)
            .font(.callout)
            .foregroundStyle(color ?? .secondary)
    }

    /// rateControlName labels a rate control.
    static func rateControlName(_ rc: String) -> String {
        switch rc {
        case RateControl.quality: "Constant quality"
        case RateControl.cbr: "Constant bitrate"
        case RateControl.vbr: "Average bitrate"
        case RateControl.twopass: "Two-pass"
        default: rc
        }
    }

    /// hertz labels a sample rate.
    static func hertz(_ rate: Int) -> String {
        rate % 1000 == 0 ? "\(rate / 1000) kHz" : String(format: "%.1f kHz", Double(rate) / 1000)
    }

    /// rate labels a frame rate without trailing zeros.
    static func rate(_ fps: Double) -> String {
        fps == fps.rounded() ? String(Int(fps)) : String(format: "%.3g", fps)
    }
}

/// BitrateField edits one bitrate as typed kilobits per second, with a menu of common values that
/// fill the same field.
struct BitrateField: View {
    /// bitsPerSecond is the bitrate, or nil for the encoder's default.
    @Binding var bitsPerSecond: Int?
    /// choices are the values the menu offers.
    let choices: [Int]

    var body: some View {
        HStack(spacing: 6) {
            TextField("Automatic", value: Binding(
                get: { bitsPerSecond.map { $0 / 1000 } },
                set: { bitsPerSecond = $0.map { $0 * 1000 } }), format: .number.grouping(.never))
                .textFieldStyle(.roundedBorder)
                .multilineTextAlignment(.trailing)
                .frame(width: 80)
                .help("Kilobits per second. Higher is sharper and larger. Leave empty for the encoder's default.")
            Text("kb/s").foregroundStyle(.secondary)
            Menu {
                ForEach(choices, id: \.self) { b in
                    Button(BitrateField.label(b)) { bitsPerSecond = b }
                }
            } label: {
                Image(systemName: "chevron.up.chevron.down")
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .fixedSize()
            .help("Common bitrates")
        }
    }

    /// label formats a bitrate for people.
    static func label(_ b: Int) -> String {
        if b >= 1_000_000 {
            let m = Double(b) / 1_000_000
            return (m == m.rounded() ? String(Int(m)) : String(format: "%.1f", m)) + " Mb/s"
        }
        let k = Double(b) / 1000
        return (k == k.rounded() ? String(Int(k)) : String(format: "%.2f", k)) + " kb/s"
    }
}
