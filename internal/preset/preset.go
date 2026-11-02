// Package preset defines the built-in conversion targets.
package preset

import (
	"fmt"
	"slices"
)

// VideoCodec names the video format a preset produces.
type VideoCodec string

// Video codecs a preset can request.
const (
	// VideoNone drops the video stream.
	VideoNone VideoCodec = ""
	// VideoCopy passes the source video through untouched.
	VideoCopy VideoCodec = "copy"
	// VideoH264 encodes H.264.
	VideoH264 VideoCodec = "h264"
	// VideoHEVC encodes H.265.
	VideoHEVC VideoCodec = "hevc"
)

// AudioCodec names the audio format a preset produces.
type AudioCodec string

// Audio codecs a preset can request.
const (
	// AudioNone drops the audio stream.
	AudioNone AudioCodec = ""
	// AudioCopy passes the source audio through untouched.
	AudioCopy AudioCodec = "copy"
	// AudioAAC encodes AAC.
	AudioAAC AudioCodec = "aac"
	// AudioMP3 encodes MP3.
	AudioMP3 AudioCodec = "mp3"
)

// Quality is a codec-neutral quality level that each encoder maps to its own scale.
type Quality int

// Quality levels from largest file to smallest.
const (
	// QualityHigh keeps nearly all visible detail.
	QualityHigh Quality = iota + 1
	// QualityBalanced trades a little detail for a much smaller file.
	QualityBalanced
	// QualitySmall favors file size over detail.
	QualitySmall
)

// Video describes the video half of a preset.
type Video struct {
	// Codec is the output video codec, or VideoNone for audio-only output.
	Codec VideoCodec `json:"codec"`
	// MaxWidth caps the long side in pixels, so portrait video gets the same box. Zero is unbounded.
	MaxWidth int `json:"maxWidth,omitempty"`
	// MaxHeight caps the short side in pixels. Zero is unbounded.
	MaxHeight int `json:"maxHeight,omitempty"`
	// Quality is the target quality level for constant-quality encodes.
	Quality Quality `json:"quality,omitempty"`
}

// Audio describes the audio half of a preset.
type Audio struct {
	// Codec is the output audio codec, or AudioNone for silent output.
	Codec AudioCodec `json:"codec"`
	// Bitrate is the audio bitrate in bits per second.
	Bitrate int `json:"bitrate,omitempty"`
	// Channels caps the channel count. Zero keeps the source layout.
	Channels int `json:"channels,omitempty"`
}

// Preset is one named conversion target.
type Preset struct {
	// ID is the stable identifier used on the command line and in the protocol.
	ID string `json:"id"`
	// Name is the short label shown in the app.
	Name string `json:"name"`
	// Description says who the preset is for.
	Description string `json:"description"`
	// Ext is the output file extension without the dot.
	Ext string `json:"ext"`
	// Video is the video settings.
	Video Video `json:"video"`
	// Audio is the audio settings.
	Audio Audio `json:"audio"`
	// TargetBytes is the desired output size. Nonzero switches to two-pass bitrate mode.
	TargetBytes int64 `json:"targetBytes,omitempty"`
}

// HasVideo reports whether the preset produces a video stream.
func (p Preset) HasVideo() bool {
	return p.Video.Codec != VideoNone
}

// HasAudio reports whether the preset produces an audio stream.
func (p Preset) HasAudio() bool {
	return p.Audio.Codec != AudioNone
}

// WithTargetSize returns a copy of p that aims for an output of n bytes.
func (p Preset) WithTargetSize(n int64) Preset {
	p.TargetBytes = n
	return p
}

// Validate reports whether p is internally consistent.
func (p Preset) Validate() error {
	switch {
	case p.ID == "":
		return fmt.Errorf("%w: empty id", ErrInvalid)
	case p.Ext == "":
		return fmt.Errorf("%w: %s: empty extension", ErrInvalid, p.ID)
	case !p.HasVideo() && !p.HasAudio():
		return fmt.Errorf("%w: %s: no video and no audio", ErrInvalid, p.ID)
	case p.TargetBytes < 0:
		return fmt.Errorf("%w: %s: negative target size", ErrInvalid, p.ID)
	case p.TargetBytes > 0 && p.Video.Codec != VideoH264 && p.Video.Codec != VideoHEVC:
		return fmt.Errorf("%w: %s: target size needs an encoded video stream", ErrInvalid, p.ID)
	case p.Video.Codec == VideoCopy && (p.Video.MaxWidth > 0 || p.Video.MaxHeight > 0):
		return fmt.Errorf("%w: %s: cannot resize copied video", ErrInvalid, p.ID)
	case p.Audio.Codec == AudioCopy && p.Audio.Channels > 0:
		return fmt.Errorf("%w: %s: cannot downmix copied audio", ErrInvalid, p.ID)
	}
	return nil
}

// TargetSizeID is the ID of the preset that needs a size from the caller.
const TargetSizeID = "target-size"

// builtin lists the presets in display order.
var builtin = []Preset{ //nolint:gochecknoglobals // Read-only table, copied on every access.
	{
		ID: "apple", Name: "Apple devices", Ext: "mp4",
		Description: "HEVC that plays on any recent iPhone, iPad, Mac, or Apple TV at half the size of H.264.",
		Video:       Video{Codec: VideoHEVC, MaxWidth: 3840, MaxHeight: 2160, Quality: QualityHigh},
		Audio:       Audio{Codec: AudioAAC, Bitrate: 192_000},
	},
	{
		ID: "compatible", Name: "Plays anywhere", Ext: "mp4",
		Description: "H.264 at up to 1080p for older devices, TVs, and anything else.",
		Video:       Video{Codec: VideoH264, MaxWidth: 1920, MaxHeight: 1080, Quality: QualityHigh},
		Audio:       Audio{Codec: AudioAAC, Bitrate: 160_000, Channels: 2},
	},
	{
		ID: "web", Name: "Web and sharing", Ext: "mp4",
		Description: "H.264 at up to 1080p that starts playing before it finishes downloading.",
		Video:       Video{Codec: VideoH264, MaxWidth: 1920, MaxHeight: 1080, Quality: QualityBalanced},
		Audio:       Audio{Codec: AudioAAC, Bitrate: 128_000, Channels: 2},
	},
	{
		ID: "tiny", Name: "Tiny file", Ext: "mp4",
		Description: "HEVC at up to 720p for email and chat when size matters most.",
		Video:       Video{Codec: VideoHEVC, MaxWidth: 1280, MaxHeight: 720, Quality: QualitySmall},
		Audio:       Audio{Codec: AudioAAC, Bitrate: 96_000, Channels: 2},
	},
	{
		ID: TargetSizeID, Name: "Fit a size", Ext: "mp4",
		Description: "H.264 at up to 1080p, sized to land just under the limit you set.",
		Video:       Video{Codec: VideoH264, MaxWidth: 1920, MaxHeight: 1080},
		Audio:       Audio{Codec: AudioAAC, Bitrate: 128_000, Channels: 2},
	},
	{
		ID: "remux", Name: "Change container", Ext: "mp4",
		Description: "Moves the streams into MP4 without re-encoding. Fast and lossless.",
		Video:       Video{Codec: VideoCopy},
		Audio:       Audio{Codec: AudioCopy},
	},
	{
		ID: "audio", Name: "Audio only", Ext: "m4a",
		Description: "AAC audio for Music, podcasts, and voice memos.",
		Audio:       Audio{Codec: AudioAAC, Bitrate: 192_000},
	},
	{
		ID: "mp3", Name: "MP3", Ext: "mp3",
		Description: "MP3 audio for players and apps that want nothing else.",
		Audio:       Audio{Codec: AudioMP3, Bitrate: 192_000, Channels: 2},
	},
}

// All returns every built-in preset in display order.
func All() []Preset {
	return slices.Clone(builtin)
}

// Lookup returns the built-in preset with the given ID.
func Lookup(id string) (Preset, error) {
	for _, p := range builtin {
		if p.ID == id {
			return p, nil
		}
	}
	return Preset{}, fmt.Errorf("%w: %q", ErrUnknown, id)
}
