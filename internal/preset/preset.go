// Package preset defines the built-in conversion targets.
package preset

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/dcadolph/icelos/internal/format"
)

// Preset is one named conversion target: a point in the format catalog with a name.
type Preset struct {
	// ID is the stable identifier used on the command line and in the protocol.
	ID string `json:"id"`
	// Name is the short label shown in the app.
	Name string `json:"name"`
	// Description says who the preset is for.
	Description string `json:"description"`
	// Target is the container and the stream settings, flattened into the preset's JSON.
	format.Target
	// Custom is true for presets the user saved, as opposed to built-in ones.
	Custom bool `json:"custom,omitempty"`
}

// MarshalJSON encodes p with its derived file extension, so clients can show it without the
// catalog.
func (p Preset) MarshalJSON() ([]byte, error) {
	type plain Preset
	return json.Marshal(struct {
		plain
		Ext string `json:"ext"`
	}{plain(p), p.Ext()})
}

// WithTargetSize returns a copy of p that aims for an output of n bytes with a two-pass encode.
func (p Preset) WithTargetSize(n int64) Preset {
	p.TargetBytes = n
	if n > 0 && p.Video.Codec != "" && p.Video.Codec != format.Copy {
		p.Video.RateControl = format.RateTwoPass
		p.Video.Bitrate = 0
	}
	return p
}

// Validate reports whether p is internally consistent and the catalog can produce it.
func (p Preset) Validate() error {
	switch {
	case p.ID == "":
		return fmt.Errorf("%w: empty id", ErrInvalid)
	case !p.HasVideo() && !p.HasAudio():
		return fmt.Errorf("%w: %s: no video and no audio", ErrInvalid, p.ID)
	case p.TargetBytes < 0:
		return fmt.Errorf("%w: %s: negative target size", ErrInvalid, p.ID)
	case p.TargetBytes > 0 && (p.Video.Codec == "" || p.Video.Codec == format.Copy):
		return fmt.Errorf("%w: %s: target size needs an encoded video stream", ErrInvalid, p.ID)
	case p.Video.Codec == format.Copy && (p.Video.MaxWidth > 0 || p.Video.MaxHeight > 0):
		return fmt.Errorf("%w: %s: cannot resize copied video", ErrInvalid, p.ID)
	case p.Audio.Codec == format.Copy && p.Audio.Channels > 0:
		return fmt.Errorf("%w: %s: cannot downmix copied audio", ErrInvalid, p.ID)
	}
	if err := format.Check(p.Target); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalid, p.ID, err)
	}
	return nil
}

// TargetSizeID is the ID of the preset that needs a size from the caller.
const TargetSizeID = "target-size"

// builtin lists the presets in display order.
var builtin = []Preset{ //nolint:gochecknoglobals // Read-only table, copied on every access.
	{
		ID: "apple", Name: "Apple devices",
		Description: "HEVC that plays on any recent iPhone, iPad, Mac, or Apple TV at half the size of H.264.",
		Target: format.Target{Container: "mp4",
			Video: format.Video{Codec: "hevc", MaxWidth: 3840, MaxHeight: 2160,
				RateControl: format.RateQuality, Quality: format.QualityHigh},
			Audio: format.Audio{Codec: "aac", Bitrate: 192_000}},
	},
	{
		ID: "compatible", Name: "Plays anywhere",
		Description: "H.264 at up to 1080p for older devices, TVs, and anything else.",
		Target: format.Target{Container: "mp4",
			Video: format.Video{Codec: "h264", MaxWidth: 1920, MaxHeight: 1080,
				RateControl: format.RateQuality, Quality: format.QualityHigh},
			Audio: format.Audio{Codec: "aac", Bitrate: 160_000, Channels: 2}},
	},
	{
		ID: "web", Name: "Web and sharing",
		Description: "H.264 at up to 1080p that starts playing before it finishes downloading.",
		Target: format.Target{Container: "mp4",
			Video: format.Video{Codec: "h264", MaxWidth: 1920, MaxHeight: 1080,
				RateControl: format.RateQuality, Quality: format.QualityBalanced},
			Audio: format.Audio{Codec: "aac", Bitrate: 128_000, Channels: 2}},
	},
	{
		ID: "tiny", Name: "Tiny file",
		Description: "HEVC at up to 720p for email and chat when size matters most.",
		Target: format.Target{Container: "mp4",
			Video: format.Video{Codec: "hevc", MaxWidth: 1280, MaxHeight: 720,
				RateControl: format.RateQuality, Quality: format.QualitySmall},
			Audio: format.Audio{Codec: "aac", Bitrate: 96_000, Channels: 2}},
	},
	{
		ID: TargetSizeID, Name: "Fit a size",
		Description: "H.264 at up to 1080p, sized to land just under the limit you set.",
		Target: format.Target{Container: "mp4",
			Video: format.Video{Codec: "h264", MaxWidth: 1920, MaxHeight: 1080,
				RateControl: format.RateTwoPass},
			Audio: format.Audio{Codec: "aac", Bitrate: 128_000, Channels: 2}},
	},
	{
		ID: "remux", Name: "Change container",
		Description: "Moves the streams into MP4 without re-encoding. Fast and lossless.",
		Target: format.Target{Container: "mp4",
			Video: format.Video{Codec: format.Copy}, Audio: format.Audio{Codec: format.Copy}},
	},
	{
		ID: "audio", Name: "Audio only",
		Description: "AAC audio for Music, podcasts, and voice memos.",
		Target:      format.Target{Container: "m4a", Audio: format.Audio{Codec: "aac", Bitrate: 192_000}},
	},
	{
		ID: "mp3", Name: "MP3",
		Description: "MP3 audio for players and apps that want nothing else.",
		Target: format.Target{Container: "mp3",
			Audio: format.Audio{Codec: "mp3", Bitrate: 192_000, Channels: 2}},
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
