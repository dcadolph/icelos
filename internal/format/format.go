// Package format holds the catalog of output containers and codecs that icelos can produce.
//
// The catalog is read-only data in Go source. Presets are points in it, and Check validates a
// preset against it, so the queue, the server, and the command line all refuse the same things.
package format

import (
	"fmt"
	"math"
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
	// MaxSampleRate is the highest sample rate the encoder accepts, zero when the codec's own
	// sample rates apply.
	MaxSampleRate int `json:"maxSampleRate,omitempty"`
}

// Profile is one profile of a codec, such as H.264 High or ProRes 422 HQ.
type Profile struct {
	// ID is the profile name passed to -profile:v.
	ID string `json:"id"`
	// Name is the label shown in the app.
	Name string `json:"name"`
	// PixFmt is the pixel format the profile encodes from, empty when the codec's PixFmt applies.
	PixFmt string `json:"pixFmt,omitempty"`
}

// Mode is one fixed frame shape a codec accepts. A codec with modes encodes nothing else.
type Mode struct {
	// Name is the label shown in the app, such as "NTSC" or "1080p 145 Mb/s".
	Name string `json:"name"`
	// Profile limits the mode to one profile, empty when it applies to every profile.
	Profile string `json:"profile,omitempty"`
	// Width is the frame width in pixels.
	Width int `json:"width"`
	// Height is the frame height in pixels.
	Height int `json:"height"`
	// FPS is the frame rate the mode requires, zero when any rate is allowed.
	FPS float64 `json:"fps,omitempty"`
	// Bitrates lists the video bitrates the mode allows in bits per second, empty when any is.
	Bitrates []int `json:"bitrates,omitempty"`
	// PixFmt is the pixel format the mode requires, empty when the codec's PixFmt applies.
	PixFmt string `json:"pixFmt,omitempty"`
}

// BitrateCap limits an audio codec's bitrate at and above one sample rate. Each nonzero limit
// applies and the smallest wins.
type BitrateCap struct {
	// MinRate is the lowest sample rate the cap applies to.
	MinRate int `json:"minRate,omitempty"`
	// Total is the most bits per second for the whole stream.
	Total int `json:"total,omitempty"`
	// PerChannel is the most bits per second for each channel.
	PerChannel int `json:"perChannel,omitempty"`
	// PerSample is the most bits for each sample of each channel.
	PerSample int `json:"perSample,omitempty"`
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
	Profiles []Profile `json:"profiles,omitempty"`
	// Levels lists the levels a user may pick, empty when the codec has none.
	Levels []string `json:"levels,omitempty"`
	// Tags lists the fourcc tags a user may pick, empty when the muxer's default is the only one.
	Tags []string `json:"tags,omitempty"`
	// PixFmt is the pixel format the encoder is given, empty for audio and for encoders that pick
	// their own.
	PixFmt string `json:"pixFmt,omitempty"`
	// Modes lists the only frame shapes the codec accepts, empty when any size is allowed.
	Modes []Mode `json:"modes,omitempty"`
	// FrameRates lists the only frame rates the codec accepts, empty when any rate is allowed.
	FrameRates []float64 `json:"frameRates,omitempty"`
	// Lossless is true for codecs that keep every sample or pixel.
	Lossless bool `json:"lossless,omitempty"`
	// SampleRates lists the sample rates the codec accepts, empty when any rate in the domain is.
	SampleRates []int `json:"sampleRates,omitempty"`
	// MaxChannels is the most channels the codec carries, zero when any layout in the domain is.
	MaxChannels int `json:"maxChannels,omitempty"`
	// Bitrates lists the only audio bitrates the codec accepts, empty when any up to the caps is.
	Bitrates []int `json:"bitrates,omitempty"`
	// BitrateCaps limit the audio bitrate by sample rate and channel count.
	BitrateCaps []BitrateCap `json:"bitrateCaps,omitempty"`
	// BitDepths lists the sample bit depths a lossless audio codec writes.
	BitDepths []int `json:"bitDepths,omitempty"`
}

// Flags are muxer settings a container always needs.
type Flags struct {
	// Faststart moves the index to the front of the file so playback can start before download.
	Faststart bool `json:"faststart,omitempty"`
	// HVC1Tag tags HEVC as hvc1, which QuickTime and Apple devices require.
	HVC1Tag bool `json:"hvc1Tag,omitempty"`
	// M2TS writes Blu-ray style 192-byte transport packets.
	M2TS bool `json:"m2ts,omitempty"`
	// Shortest ends the file with its shortest stream, for muxers that refuse a last video frame
	// without its full share of audio.
	Shortest bool `json:"shortest,omitempty"`
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
	// SampleRates lists the only audio sample rates the muxer accepts, empty when the codec decides.
	SampleRates []int `json:"sampleRates,omitempty"`
	// Channels is the only audio channel count the muxer accepts, zero when the codec decides.
	Channels int `json:"channels,omitempty"`
	// AudioTracks is the most audio streams the muxer accepts, zero when it takes any number.
	AudioTracks int `json:"audioTracks,omitempty"`
	// Modes lists the only frame shapes the container's players accept, such as the DVD sizes in
	// VOB, empty when the codec decides.
	Modes []Mode `json:"modes,omitempty"`
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
	c.SampleRates = slices.Clone(c.SampleRates)
	c.Modes = cloneModes(c.Modes)
	return c
}

// cloneModes returns a copy of modes that shares no slices with the catalog.
func cloneModes(modes []Mode) []Mode {
	out := slices.Clone(modes)
	for i := range out {
		out[i].Bitrates = slices.Clone(out[i].Bitrates)
	}
	return out
}

// clone returns a copy of c that shares no slices with the catalog.
func (c Codec) clone() Codec {
	c.Encoders = slices.Clone(c.Encoders)
	c.RateControls = slices.Clone(c.RateControls)
	c.Profiles = slices.Clone(c.Profiles)
	c.Levels = slices.Clone(c.Levels)
	c.Tags = slices.Clone(c.Tags)
	c.Modes = cloneModes(c.Modes)
	c.FrameRates = slices.Clone(c.FrameRates)
	c.SampleRates = slices.Clone(c.SampleRates)
	c.Bitrates = slices.Clone(c.Bitrates)
	c.BitrateCaps = slices.Clone(c.BitrateCaps)
	c.BitDepths = slices.Clone(c.BitDepths)
	return c
}

// ProfileByID returns the codec's profile with the given ID.
func (c Codec) ProfileByID(id string) (Profile, bool) {
	for _, p := range c.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// Match returns the mode that a profile, frame size, frame rate, and video bitrate fit. A zero
// width, height, frame rate, or bitrate matches any value, and the first fitting mode in table
// order wins. A codec with no modes for the profile accepts anything and returns a zero Mode.
func (c Codec) Match(profile string, width, height int, fps float64, bitrate int) (Mode, error) {
	return matchMode(c.Modes, c.ID, profile, width, height, fps, bitrate)
}

// Match returns the container mode that a frame size and frame rate fit, with the same rules as
// Codec.Match. A container with no modes accepts anything and returns a zero Mode.
func (c Container) Match(width, height int, fps float64) (Mode, error) {
	return matchMode(c.Modes, c.ID, "", width, height, fps, 0)
}

// Fixed reports whether the codec encodes only fixed frame shapes for the profile.
func (c Codec) Fixed(profile string) bool {
	return slices.ContainsFunc(c.Modes, func(m Mode) bool {
		return m.Profile == "" || m.Profile == profile
	})
}

// matchMode returns the first mode in modes that fits, skipping modes tied to another profile.
// The name identifies the codec or container in the error.
func matchMode(modes []Mode, name, profile string, width, height int, fps float64, bitrate int) (Mode, error) {
	fixed := false
	for _, m := range modes {
		if m.Profile != "" && m.Profile != profile {
			continue
		}
		fixed = true
		switch {
		case width != 0 && width != m.Width, height != 0 && height != m.Height:
		case fps != 0 && m.FPS != 0 && !SameRate(fps, m.FPS):
		case bitrate != 0 && len(m.Bitrates) != 0 && !slices.Contains(m.Bitrates, bitrate):
		default:
			return m, nil
		}
	}
	if !fixed {
		return Mode{}, nil
	}
	return Mode{}, fmt.Errorf("%w: %s: %dx%d at %g fps and %d b/s", ErrMode, name, width, height,
		fps, bitrate)
}

// AllowsFrameRate reports whether the codec encodes at fps. Zero, meaning same as source, is
// always allowed.
func (c Codec) AllowsFrameRate(fps float64) bool {
	if fps == 0 || len(c.FrameRates) == 0 {
		return true
	}
	return slices.ContainsFunc(c.FrameRates, func(r float64) bool { return SameRate(r, fps) })
}

// MaxBitrate returns the highest audio bitrate the codec accepts at a sample rate and channel
// count, or zero when the codec has no cap.
func (c Codec) MaxBitrate(sampleRate, channels int) int {
	best := -1
	for i, b := range c.BitrateCaps {
		if b.MinRate <= sampleRate && (best < 0 || b.MinRate > c.BitrateCaps[best].MinRate) {
			best = i
		}
	}
	if best < 0 {
		return 0
	}
	b := c.BitrateCaps[best]
	limit := 0
	for _, v := range []int{b.Total, b.PerChannel * channels, b.PerSample * sampleRate * channels} {
		if v > 0 && (limit == 0 || v < limit) {
			limit = v
		}
	}
	return limit
}

// SameRate reports whether two frame rates are equal, treating 29.97 and 30000/1001 as one.
func SameRate(a, b float64) bool {
	return math.Abs(a-b) < 0.01
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
