import IcelosCore
import SwiftUI

/// Inspector edits the window settings, or the selected file's own settings, live as the user
/// changes them. Nothing about it is modal.
struct Inspector: View {
    /// model is the window state.
    @Bindable var model: Converter
    /// presetName is the name typed for Save as Preset.
    @State private var presetName = ""
    /// naming is true while the Save as Preset field shows.
    @State private var naming = false

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            form
            Divider()
            footer
        }
        // A small ideal height keeps the form's full height from becoming the window's size.
        .frame(minHeight: 0, idealHeight: 200, maxHeight: .infinity)
    }

    /// form is the grouped settings form, drawn on the inspector's own background, or a spinner
    /// until the catalog arrives.
    @ViewBuilder private var form: some View {
        Group {
            if let catalog = model.catalog, let current = model.inspected {
                SettingsForm(catalog: catalog,
                             draft: Binding(get: { model.inspected ?? current }, set: { model.editInspected($0) }),
                             targetSize: $model.targetSize, passthrough: $model.passthrough,
                             copyNote: CopyNote.text(model.selectedPlan, passthrough: model.passthrough)) {
                    if case .item(let id, _, _) = model.inspectorScope,
                       let item = model.items.first(where: { $0.id == id }), let info = item.info {
                        StreamsSection(model: model, item: item, info: info)
                    }
                }
            } else {
                ProgressView()
                    .controlSize(.small)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .frame(maxHeight: .infinity)
    }

    /// header says what the inspector edits, with the switch between a file's own settings and the
    /// window settings when a file is selected.
    private var header: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title)
                .font(.title3.weight(.semibold))
                .lineLimit(2)
                .truncationMode(.middle)
            if let summary {
                Text(summary)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if case .item(let id, let name, let usesWindow) = model.inspectorScope {
                Toggle("Use window settings", isOn: Binding(
                    get: { usesWindow },
                    set: { model.setUsesWindowSettings(id, $0) }))
                    .toggleStyle(.switch)
                    .controlSize(.small)
                    .help("Turn off to give \(name) its own settings. The other files keep the window settings.")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 16)
        .padding(.vertical, 14)
    }

    /// title names what the form edits.
    private var title: String {
        if case .item(_, let name, false) = model.inspectorScope { return "Override for \(name)" }
        return "Window Settings"
    }

    /// summary describes the inspected settings in a sentence.
    private var summary: String? {
        guard let p = model.inspected else { return nil }
        let line = model.catalog?.summary(p) ?? p.ext.uppercased()
        if case .item = model.inspectorScope { return line }
        if model.presetID.isEmpty { return line }
        return model.isEdited ? "\(p.name), edited. \(line)" : "\(p.name). \(line)"
    }

    /// footer holds Reset and Save as Preset, and the latest preset problem.
    private var footer: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let error = model.presetError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .font(.callout)
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack(spacing: 8) {
                Button("Reset") {
                    if let original = model.inspectedOriginal { model.editInspected(original) }
                }
                .help("Go back to the settings of the chosen preset or format.")
                .disabled(model.inspected == nil || model.inspected == model.inspectedOriginal)
                Spacer(minLength: 0)
                if naming {
                    TextField("Preset name", text: $presetName)
                        .textFieldStyle(.roundedBorder)
                        .frame(maxWidth: 140)
                        .onSubmit(save)
                        .onExitCommand { naming = false }
                    Button("Save", action: save)
                        .disabled(presetName.trimmingCharacters(in: .whitespaces).isEmpty)
                } else {
                    Button("Save as Preset…") { naming = true }
                        .help("Keep these settings as a preset in the Custom section.")
                        .disabled(model.inspected == nil)
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }

    /// save sends the inspected settings to be saved as a custom preset.
    private func save() {
        let name = presetName.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty, let p = model.inspected else { return }
        model.savePreset(p, name: name)
        naming = false
        presetName = ""
    }
}

/// SettingsForm edits one set of conversion settings with controls built from the catalog, so a
/// new codec in the backend needs no change here. Extra sections, such as a file's streams, follow
/// the audio section.
struct SettingsForm<Extra: View>: View {
    /// catalog supplies the containers, codecs, and value lists.
    let catalog: Catalog
    /// draft is the settings being edited, applied as each control changes.
    @Binding var draft: Preset
    /// targetSize is the size the target-size preset aims for.
    @Binding var targetSize: String
    /// passthrough copies streams that already match instead of encoding them.
    @Binding var passthrough: Bool
    /// copyNote says what passthrough does for the selected file.
    let copyNote: String
    /// extra holds the sections after audio.
    @ViewBuilder let extra: Extra

    var body: some View {
        Form {
            formatSection
            if shown.contains(.videoEncoder) { videoSection }
            audioSection
            extra
        }
        .formStyle(.grouped)
    }

    /// shown is the set of controls the catalog rules give the draft.
    private var shown: Set<InspectorControl> { Set(catalog.inspectorControls(for: draft)) }

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
            if shown.contains(.fitUnder) {
                LabeledContent("Fit under") {
                    TextField("25MB", text: $targetSize)
                        .textFieldStyle(.roundedBorder)
                        .multilineTextAlignment(.trailing)
                        .frame(width: 90)
                }
                .help("The size each file should come in under, such as 25MB or 1.5GB.")
            }
            Toggle("Copy streams that already match", isOn: $passthrough)
                .help("A stream already in the target codec, at the same size and rate, is copied as it is: no quality loss and much faster.")
        } footer: {
            Text(copyNote)
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
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
                    if shown.contains(.modes) {
                        modePicker(codec, container)
                    } else if shown.contains(.resolution) {
                        resolutionPicker
                    }
                    if shown.contains(.frameRate) {
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
        if shown.contains(.frameSize) {
            LabeledContent("Frame size") {
                HStack(spacing: 6) {
                    dimensionField($draft.video.width)
                    Text("×")
                    dimensionField($draft.video.height)
                }
            }
            if let problem = sizeProblem {
                note(problem, color: .red)
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
        if shown.contains(.rateControl), let rc = draft.video.rateControl {
            Picker("Rate control", selection: Binding(
                get: { rc },
                set: { draft.video = catalog.withRateControl(draft.video, $0, keeping: draft.video) })) {
                ForEach(controls, id: \.self) { Text(Self.rateControlName($0)).tag($0) }
            }
            .help("Constant quality lets the size vary. Bitrate modes aim for a data rate. Two-pass is slower and more exact.")
            if shown.contains(.quality) {
                Picker("Quality", selection: Binding(
                    get: { draft.video.quality ?? QualityLevel.high },
                    set: { draft.video.quality = $0 })) {
                    Text("High").tag(QualityLevel.high)
                    Text("Balanced").tag(QualityLevel.balanced)
                    Text("Small").tag(QualityLevel.small)
                }
                .pickerStyle(.segmented)
                .help("High keeps nearly all detail. Small gives the smallest file.")
            } else if shown.contains(.videoBitrate) {
                LabeledContent("Bitrate") {
                    BitrateField(bitsPerSecond: $draft.video.bitrate, choices: catalog.domains.videoBitrates)
                }
            } else if rc == RateControl.twopass {
                note("Two passes aim for the size set under Fit under.")
            }
        }
    }

    /// codecOptions offers the codec's profiles, levels, and tags.
    @ViewBuilder private func codecOptions(_ codec: Codec, _ container: Container) -> some View {
        if shown.contains(.profile), let profiles = codec.profiles {
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
        if shown.contains(.level), let levels = codec.levels {
            Picker("Level", selection: Binding(
                get: { draft.video.level ?? "" },
                set: { draft.video.level = $0.isEmpty ? nil : $0 })) {
                Text("Automatic").tag("")
                ForEach(levels, id: \.self) { Text($0).tag($0) }
            }
            .help("Caps resolution and data rate for device compatibility. Automatic picks one that fits.")
        }
        if shown.contains(.tag), let tags = codec.tags {
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
                if shown.contains(.includeAudio) {
                    Toggle("Include audio", isOn: Binding(
                        get: { !draft.audio.codec.isEmpty },
                        set: { on in
                            let id = on ? (catalog.defaultCodec("audio", in: container)?.id ?? "") : ""
                            draft = catalog.settingAudioCodec(draft, to: id)
                        }))
                        .help("Turn off to write a silent video.")
                }
                if shown.contains(.audioEncoder) {
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
        if shown.contains(.sampleRate) {
            Picker("Sample rate", selection: Binding(
                get: { draft.audio.sampleRate ?? 0 },
                set: { draft.audio.sampleRate = $0 == 0 ? nil : $0; clampAudioBitrate(codec) })) {
                if container.sampleRates?.isEmpty != false { Text("Same as source").tag(0) }
                ForEach(catalog.sampleRates(for: codec, in: container), id: \.self) { Text(Self.hertz($0)).tag($0) }
            }
            .help("Samples per second. Use 48 kHz for video and 44.1 kHz for music.")
        }
        if shown.contains(.channels) {
            Picker("Channels", selection: Binding(
                get: { draft.audio.channels ?? 0 },
                set: { draft.audio.channels = $0 == 0 ? nil : $0; clampAudioBitrate(codec) })) {
                if container.channels == nil { Text("Same as source").tag(0) }
                ForEach(catalog.channelLayouts(for: codec, in: container), id: \.count) { Text($0.name).tag($0.count) }
            }
            .help("Mono, stereo, or surround. Same as source keeps the original layout.")
        }
        let choices = catalog.audioBitrates(for: codec, sampleRate: draft.audio.sampleRate,
                                            channels: draft.audio.channels)
        if shown.contains(.fixedAudioBitrate), let fixed = codec.bitrates {
            Picker("Bitrate", selection: Binding(
                get: { draft.audio.bitrate ?? fixed.max() ?? 0 },
                set: { draft.audio.bitrate = $0 })) {
                ForEach(fixed, id: \.self) { Text(BitrateField.label($0)).tag($0) }
            }
        } else if shown.contains(.audioBitrate) {
            LabeledContent("Bitrate") {
                BitrateField(bitsPerSecond: $draft.audio.bitrate, choices: choices)
            }
            let cap = catalog.maxAudioBitrate(for: codec, sampleRate: draft.audio.sampleRate,
                                              channels: draft.audio.channels)
            if cap > 0, let b = draft.audio.bitrate, b > cap {
                note("\(codec.name) tops out at \(BitrateField.label(cap)) here.", color: .red)
            }
        }
        if shown.contains(.bitDepth), let depths = codec.bitDepths {
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

    /// note is a secondary line of explanation.
    private func note(_ text: String, color: Color? = nil) -> some View {
        Text(text)
            .font(.callout)
            .foregroundStyle(color.map(AnyShapeStyle.init) ?? AnyShapeStyle(.secondary))
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
            .accessibilityLabel("Common bitrates")
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

/// StreamsSection chooses the tracks and extras one file's output carries: its audio tracks, added
/// audio files, subtitles, chapters, and metadata.
struct StreamsSection: View {
    /// model receives the changes.
    let model: Converter
    /// item is the selected file.
    let item: Item
    /// info is the file's probe result.
    let info: MediaInfo

    var body: some View {
        Section("Streams") {
            audioTracks
            externalAudio
            subtitles
            Toggle("Keep chapters", isOn: binding(\.dropChapters, inverted: true))
                .help("Carry the source's chapter marks into the output.")
            Toggle("Keep metadata", isOn: binding(\.dropMetadata, inverted: true))
                .help("Carry the source's title, date, and other tags into the output.")
            let chapters = info.chapters?.count ?? 0
            Toggle(chapters > 0 ? "Split into \(chapters) files by chapter" : "Split by chapters",
                   isOn: Binding(get: { item.splitChapters }, set: { model.setSplitChapters(item.id, $0) }))
                .disabled(chapters == 0)
                .help(chapters == 0 ? "This file has no chapters." : "Write one file per chapter, named after it.")
        }
    }

    /// audioTracks lists the source's audio tracks with a switch each.
    @ViewBuilder private var audioTracks: some View {
        let tracks = info.audioStreams
        if tracks.count > 1 {
            let kept = Set(item.streams.tracks(count: tracks.count))
            ForEach(tracks.indices, id: \.self) { i in
                Toggle(StreamsSection.label(tracks[i], position: i), isOn: Binding(
                    get: { kept.contains(i) },
                    set: { model.setAudioTrack(item.id, i, on: $0) }))
                    .disabled(kept == [i])
                    .help(kept == [i] ? "At least one audio track stays." : "Carry this audio track into the output.")
            }
        }
    }

    /// externalAudio lists the added audio files and offers to add more.
    @ViewBuilder private var externalAudio: some View {
        let files = item.streams.externalAudio ?? []
        ForEach(files.indices, id: \.self) { i in
            fileRow(files[i], kind: "audio") { model.removeExternalAudio(item.id, at: i) }
        }
        Button("Add Audio Track…") {
            MediaFiles.chooseAudio { model.addExternalAudio(item.id, $0) }
        }
        .disabled(files.count >= Streams.maxExternalAudio)
        .help("Add up to \(Streams.maxExternalAudio) audio files, such as a dub or a commentary, as extra tracks.")
    }

    /// subtitles picks the subtitle mode, the track, and an added subtitle file.
    @ViewBuilder private var subtitles: some View {
        let modes = model.subtitleModes(item)
        let sub = item.streams.subtitles ?? Subtitles()
        Picker("Subtitles", selection: Binding(
            get: { item.streams.subtitleMode },
            set: { mode in
                var s = item.streams
                var next = s.subtitles ?? Subtitles()
                next.mode = mode
                s.subtitles = next
                model.setStreams(item.id, s)
            })) {
            ForEach(modes, id: \.self) { Text(StreamsSection.name($0)).tag($0) }
        }
        .help(modes == [.none] ? "This file has no subtitles. Add a subtitle file to use one." :
              "Keep carries every track the format holds. Pick carries one. Burn in draws one into the picture.")
        let own = info.subtitleStreams
        if [.pick, .burn].contains(item.streams.subtitleMode), sub.external == nil, own.count > 1 {
            Picker("Subtitle track", selection: Binding(
                get: { sub.track ?? 0 },
                set: { track in
                    var s = item.streams
                    s.subtitles?.track = track
                    model.setStreams(item.id, s)
                })) {
                ForEach(own.indices, id: \.self) { Text(StreamsSection.label(own[$0], position: $0)).tag($0) }
            }
            .help("The subtitle track to carry or burn in.")
        }
        if let file = sub.external {
            fileRow(file, kind: "subtitle") { model.setSubtitleFile(item.id, nil) }
        } else {
            Button("Add Subtitle File…") {
                MediaFiles.chooseSubtitle { model.setSubtitleFile(item.id, $0) }
            }
            .help("Use an SRT, ASS, SSA, or VTT file. Pick and burn in use it in place of the file's own tracks.")
        }
    }

    /// fileRow shows an added file with a button that removes it.
    private func fileRow(_ path: String, kind: String, remove: @escaping () -> Void) -> some View {
        HStack(spacing: 6) {
            Text(URL(fileURLWithPath: path).lastPathComponent)
                .lineLimit(1)
                .truncationMode(.middle)
            Spacer(minLength: 4)
            Button(action: remove) {
                Image(systemName: "minus.circle")
            }
            .buttonStyle(.borderless)
            .help("Remove this \(kind) file.")
            .accessibilityLabel("Remove \(URL(fileURLWithPath: path).lastPathComponent)")
        }
    }

    /// binding edits one optional flag of the file's streams, inverted for the keep switches.
    private func binding(_ key: WritableKeyPath<Streams, Bool?>, inverted: Bool) -> Binding<Bool> {
        Binding(
            get: { (item.streams[keyPath: key] ?? false) != inverted },
            set: { on in
                var s = item.streams
                s[keyPath: key] = on != inverted
                model.setStreams(item.id, s)
            })
    }

    /// label names a track by its language, title, codec, and channels.
    static func label(_ s: MediaStream, position: Int) -> String {
        var parts: [String] = []
        if let lang = s.tags?.language, !lang.isEmpty, lang != "und" {
            parts.append(Locale.current.localizedString(forLanguageCode: lang) ?? lang.uppercased())
        }
        if let title = s.tags?.title, !title.isEmpty { parts.append(title) }
        if let codec = s.codec { parts.append(codec.uppercased()) }
        if let ch = s.channels, ch > 0 { parts.append(ch == 1 ? "mono" : ch == 2 ? "stereo" : "\(ch) ch") }
        return "\(position + 1). " + parts.joined(separator: ", ")
    }

    /// name labels a subtitle mode.
    static func name(_ mode: SubtitleMode) -> String {
        switch mode {
        case .none: "None"
        case .keep: "Keep all"
        case .pick: "Pick one"
        case .burn: "Burn in"
        }
    }
}
