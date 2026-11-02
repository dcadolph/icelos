// Package format holds the catalog of output containers and codecs that icelos can produce.
//
// The catalog is read-only data in Go source. Presets are points in it, and Check validates a
// preset against it, so the queue, the server, and the command line all refuse the same things.
package format

import (
	"fmt"
	"slices"
)

// Kind says what a stream or container carries.
type Kind string

// Stream and container kinds.
const (
	// KindVideo is a video stream, or a container whose main stream is video.
	KindVideo Kind = "video"
	// KindAudio is an audio stream, or an audio-only container.
	KindAudio Kind = "audio"
	// KindSubtitle is a subtitle stream.
	KindSubtitle Kind = "subtitle"
)

// RateControl names how an encoder spends bits.
type RateControl string

// Rate controls a codec can support.
const (
	// RateQuality holds a constant quality and lets the bitrate float.
	RateQuality RateControl = "quality"
	// RateCBR holds a constant bitrate.
	RateCBR RateControl = "cbr"
	// RateVBR targets an average bitrate.
	RateVBR RateControl = "vbr"
	// RateTwoPass analyzes the file once and then encodes to an exact average bitrate.
	RateTwoPass RateControl = "twopass"
)

// Encoder is one ffmpeg encoder that produces a codec.
type Encoder struct {
	// Name is the ffmpeg encoder name, as "ffmpeg -encoders" lists it.
	Name string `json:"name"`
	// Hardware is true for encoders that run on the Media Engine through VideoToolbox.
	Hardware bool `json:"hardware,omitempty"`
}

// Codec is one output codec and the rules its encoders follow.
type Codec struct {
	// ID is the ffmpeg codec name, such as "h264" or "aac".
	ID string `json:"id"`
	// Name is the label shown in the app.
	Name string `json:"name"`
	// Kind is the stream kind the codec produces.
	Kind Kind `json:"kind"`
	// Encoders lists the encoders for this codec in order of preference.
	Encoders []Encoder `json:"encoders"`
	// RateControls lists the rate controls the encoders support.
	RateControls []RateControl `json:"rateControls,omitempty"`
	// Profiles lists the profiles a user may pick, empty when the codec has none.
	Profiles []string `json:"profiles,omitempty"`
	// Levels lists the levels a user may pick, empty when the codec has none.
	Levels []string `json:"levels,omitempty"`
	// PixFmt is the pixel format the encoder is given, empty for audio and for encoders that pick
	// their own.
	PixFmt string `json:"pixFmt,omitempty"`
}

// Flags are muxer settings a container always needs.
type Flags struct {
	// Faststart moves the index to the front of the file so playback can start before download.
	Faststart bool `json:"faststart,omitempty"`
	// HVC1Tag tags HEVC as hvc1, which QuickTime and Apple devices require.
	HVC1Tag bool `json:"hvc1Tag,omitempty"`
	// M2TS writes Blu-ray style 192-byte transport packets.
	M2TS bool `json:"m2ts,omitempty"`
}

// Container is one output file format.
type Container struct {
	// ID is the stable identifier presets use, such as "mp4" or "m4r".
	ID string `json:"id"`
	// Name is the label shown in the app.
	Name string `json:"name"`
	// Ext is the file extension without the dot.
	Ext string `json:"ext"`
	// Muxer is the ffmpeg muxer name passed to -f.
	Muxer string `json:"muxer"`
	// Kind is KindVideo for video containers and KindAudio for audio-only ones.
	Kind Kind `json:"kind"`
	// Video lists the video codec IDs the container may carry, in order of preference.
	Video []string `json:"video,omitempty"`
	// Audio lists the audio codec IDs the container may carry, in order of preference.
	Audio []string `json:"audio,omitempty"`
	// Subtitles lists the soft subtitle codec IDs the container may carry, empty when it has none.
	Subtitles []string `json:"subtitles,omitempty"`
	// Flags are the muxer settings the container always needs.
	Flags Flags `json:"flags"`
}

// Allows reports whether the container may carry codec id of the given kind.
func (c Container) Allows(kind Kind, id string) bool {
	switch kind {
	case KindVideo:
		return slices.Contains(c.Video, id)
	case KindAudio:
		return slices.Contains(c.Audio, id)
	case KindSubtitle:
		return slices.Contains(c.Subtitles, id)
	}
	return false
}

// clone returns a copy of c that shares no slices with the catalog.
func (c Container) clone() Container {
	c.Video = slices.Clone(c.Video)
	c.Audio = slices.Clone(c.Audio)
	c.Subtitles = slices.Clone(c.Subtitles)
	return c
}

// clone returns a copy of c that shares no slices with the catalog.
func (c Codec) clone() Codec {
	c.Encoders = slices.Clone(c.Encoders)
	c.RateControls = slices.Clone(c.RateControls)
	c.Profiles = slices.Clone(c.Profiles)
	c.Levels = slices.Clone(c.Levels)
	return c
}

// Containers returns every container in catalog order. The result is a copy the caller may modify.
func Containers() []Container {
	out := make([]Container, len(containers))
	for i, c := range containers {
		out[i] = c.clone()
	}
	return out
}

// ContainerByID returns the container with the given ID.
func ContainerByID(id string) (Container, error) {
	for _, c := range containers {
		if c.ID == id {
			return c.clone(), nil
		}
	}
	return Container{}, fmt.Errorf("%w: %q", ErrUnknownContainer, id)
}

// Codecs returns every codec in catalog order. The result is a copy the caller may modify.
func Codecs() []Codec {
	out := make([]Codec, len(codecs))
	for i, c := range codecs {
		out[i] = c.clone()
	}
	return out
}

// CodecByID returns the codec with the given ID.
func CodecByID(id string) (Codec, error) {
	for _, c := range codecs {
		if c.ID == id {
			return c.clone(), nil
		}
	}
	return Codec{}, fmt.Errorf("%w: %q", ErrUnknownCodec, id)
}

// codecs is the catalog of output codecs.
var codecs []Codec //nolint:gochecknoglobals // Read-only catalog data.
