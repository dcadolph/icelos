package format

// Copy is the codec ID that passes a source stream through untouched.
const Copy = "copy"

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

// Video is the video half of a target: what the settings dialog edits for the video stream.
type Video struct {
	// Codec is a catalog codec ID, Copy to pass the source through, or empty to drop video.
	Codec string `json:"codec"`
	// Width is the exact output width in pixels. Zero keeps the source width, or follows the
	// aspect ratio when only Height is set.
	Width int `json:"width,omitempty"`
	// Height is the exact output height in pixels. Zero keeps the source height, or follows the
	// aspect ratio when only Width is set.
	Height int `json:"height,omitempty"`
	// MaxWidth caps the long side in pixels, so portrait video gets the same box. Zero is unbounded.
	MaxWidth int `json:"maxWidth,omitempty"`
	// MaxHeight caps the short side in pixels. Zero is unbounded.
	MaxHeight int `json:"maxHeight,omitempty"`
	// FPS is the output frame rate. Zero keeps the source rate.
	FPS float64 `json:"fps,omitempty"`
	// RateControl is how the encoder spends bits, empty for codecs that have no choice.
	RateControl RateControl `json:"rateControl,omitempty"`
	// Bitrate is the video bitrate in bits per second for bitrate rate controls and fixed modes.
	Bitrate int `json:"bitrate,omitempty"`
	// Profile is a profile ID from the codec, empty for the encoder default.
	Profile string `json:"profile,omitempty"`
	// Level is a level from the codec, empty for the encoder default.
	Level string `json:"level,omitempty"`
	// Tag is a fourcc tag from the codec, empty for the muxer default.
	Tag string `json:"tag,omitempty"`
	// Quality is the target quality level for the quality rate control.
	Quality Quality `json:"quality,omitempty"`
}

// Audio is the audio half of a target: what the settings dialog edits for the audio stream.
type Audio struct {
	// Codec is a catalog codec ID, Copy to pass the source through, or empty to drop audio.
	Codec string `json:"codec"`
	// Bitrate is the audio bitrate in bits per second. Zero is the encoder default.
	Bitrate int `json:"bitrate,omitempty"`
	// SampleRate is the output sample rate in hertz. Zero keeps the source rate.
	SampleRate int `json:"sampleRate,omitempty"`
	// Channels caps the channel count. Zero keeps the source layout.
	Channels int `json:"channels,omitempty"`
	// BitDepth is the sample bit depth for lossless codecs. Zero is the codec default.
	BitDepth int `json:"bitDepth,omitempty"`
}

// Target is one point in the catalog: a container and the settings for each stream.
type Target struct {
	// Container is the catalog container ID.
	Container string `json:"container"`
	// Video is the video settings.
	Video Video `json:"video"`
	// Audio is the audio settings.
	Audio Audio `json:"audio"`
	// TargetBytes is the desired output size. Nonzero needs the two-pass rate control.
	TargetBytes int64 `json:"targetBytes,omitempty"`
}

// Ext returns the output file extension without the dot, or empty for an unknown container.
func (t Target) Ext() string {
	for _, c := range containers {
		if c.ID == t.Container {
			return c.Ext
		}
	}
	return ""
}

// HasVideo reports whether the target produces a video stream.
func (t Target) HasVideo() bool {
	return t.Video.Codec != ""
}

// HasAudio reports whether the target produces an audio stream.
func (t Target) HasAudio() bool {
	return t.Audio.Codec != ""
}
