import Foundation

/// InspectorControl is one control the settings inspector can show.
public enum InspectorControl: String, CaseIterable, Sendable {
    case format
    case fitUnder
    case videoEncoder
    case resolution
    case frameSize
    case modes
    case frameRate
    case rateControl
    case quality
    case videoBitrate
    case profile
    case level
    case tag
    case includeAudio
    case audioEncoder
    case sampleRate
    case channels
    case audioBitrate
    case fixedAudioBitrate
    case bitDepth

    /// domains lists the catalog option domains the control offers its values from.
    public var domains: [DomainKey] {
        switch self {
        case .resolution: [.resolutions]
        case .frameSize: [.minDimension, .maxDimension]
        case .frameRate: [.frameRates]
        case .videoBitrate: [.videoBitrates]
        case .sampleRate: [.sampleRates]
        case .channels: [.channels]
        case .audioBitrate: [.audioBitrates]
        default: []
        }
    }
}

/// DomainKey names one field of the catalog's option domains.
public enum DomainKey: String, CaseIterable, Sendable {
    case resolutions
    case minDimension
    case maxDimension
    case frameRates
    case videoBitrates
    case audioBitrates
    case sampleRates
    case channels
}

extension Catalog {
    /// inspectorControls lists the controls the inspector shows for draft, in display order.
    public func inspectorControls(for draft: Preset) -> [InspectorControl] {
        var out: [InspectorControl] = [.format]
        if draft.needsSize { out.append(.fitUnder) }
        guard let box = container(id: draft.container) else { return out }
        if box.kind == "video" {
            out.append(.videoEncoder)
            if let vc = codec(id: draft.video.codec) {
                out += videoControls(vc, in: box, draft: draft)
            }
            out.append(.includeAudio)
        }
        if !draft.audio.codec.isEmpty {
            out.append(.audioEncoder)
            if let ac = codec(id: draft.audio.codec) {
                out += audioControls(ac, draft: draft)
            }
        }
        return out
    }

    /// videoControls lists the video controls after the encoder for codec vc in box.
    private func videoControls(_ vc: Codec, in box: Container, draft: Preset) -> [InspectorControl] {
        var out: [InspectorControl] = []
        if isFixed(vc, profile: draft.video.profile, in: box) {
            out.append(.modes)
        } else {
            out.append(.resolution)
            if draft.video.width != nil || draft.video.height != nil { out.append(.frameSize) }
        }
        if !modes(for: vc, profile: draft.video.profile, in: box).contains(where: { $0.fps != nil }) {
            out.append(.frameRate)
        }
        if !(vc.rateControls ?? []).isEmpty, let rc = draft.video.rateControl {
            out.append(.rateControl)
            switch rc {
            case RateControl.quality: out.append(.quality)
            case RateControl.twopass where draft.needsSize: break
            default: out.append(.videoBitrate)
            }
        }
        if !(vc.profiles ?? []).isEmpty { out.append(.profile) }
        if !(vc.levels ?? []).isEmpty { out.append(.level) }
        if !(vc.tags ?? []).isEmpty { out.append(.tag) }
        return out
    }

    /// audioControls lists the audio controls after the encoder for codec ac.
    private func audioControls(_ ac: Codec, draft: Preset) -> [InspectorControl] {
        var out: [InspectorControl] = [.sampleRate, .channels]
        if !(ac.bitrates ?? []).isEmpty {
            out.append(.fixedAudioBitrate)
        } else if !audioBitrates(for: ac, sampleRate: draft.audio.sampleRate, channels: draft.audio.channels).isEmpty {
            out.append(.audioBitrate)
        }
        if (ac.bitDepths ?? []).count > 1 { out.append(.bitDepth) }
        return out
    }
}
