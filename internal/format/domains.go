package format

import (
	"slices"
	"strconv"
)

// Resolution is one entry in the resolution ladder. A zero width and height means same as source.
type Resolution struct {
	// ID is the stable identifier, such as "1080p" or "same".
	ID string `json:"id"`
	// Name is the label shown in the app.
	Name string `json:"name"`
	// Width is the box width in pixels, zero for same as source.
	Width int `json:"width"`
	// Height is the box height in pixels, zero for same as source.
	Height int `json:"height"`
}

// FrameRate is one entry in the frame rate list. A zero FPS means same as source.
type FrameRate struct {
	// Name is the label shown in the app.
	Name string `json:"name"`
	// FPS is the rate in frames per second, zero for same as source.
	FPS float64 `json:"fps"`
}

// ChannelLayout is one entry in the channel list.
type ChannelLayout struct {
	// Count is the number of channels.
	Count int `json:"count"`
	// Name is the label shown in the app.
	Name string `json:"name"`
}

// Domains are the value lists the settings dialog offers.
type Domains struct {
	// Resolutions is the resolution ladder, same as source first.
	Resolutions []Resolution `json:"resolutions"`
	// MinDimension is the smallest custom width or height in pixels.
	MinDimension int `json:"minDimension"`
	// MaxDimension is the largest custom width or height in pixels.
	MaxDimension int `json:"maxDimension"`
	// FrameRates is the frame rate list, same as source first.
	FrameRates []FrameRate `json:"frameRates"`
	// VideoBitrates lists the suggested video bitrates in bits per second.
	VideoBitrates []int `json:"videoBitrates"`
	// AudioBitrates lists the suggested audio bitrates in bits per second.
	AudioBitrates []int `json:"audioBitrates"`
	// SampleRates lists the audio sample rates in hertz.
	SampleRates []int `json:"sampleRates"`
	// Channels lists the channel layouts.
	Channels []ChannelLayout `json:"channels"`
}

// domains holds the option lists.
var domains = Domains{ //nolint:gochecknoglobals // Read-only catalog data.
	Resolutions: []Resolution{
		{ID: "same", Name: "Same as source"},
		{ID: "8k", Name: "8K", Width: 7680, Height: 4320},
		{ID: "4k", Name: "4K", Width: 3840, Height: 2160},
		{ID: "1080p", Name: "1080p", Width: 1920, Height: 1080},
		{ID: "720p", Name: "720p", Width: 1280, Height: 720},
		{ID: "480p", Name: "480p", Width: 854, Height: 480},
	},
	MinDimension: 32,
	MaxDimension: 8192,
	FrameRates: []FrameRate{
		{Name: "Same as source"},
		{Name: "12", FPS: 12}, {Name: "15", FPS: 15}, {Name: "20", FPS: 20},
		{Name: "23.976", FPS: 23.976}, {Name: "24", FPS: 24}, {Name: "25", FPS: 25},
		{Name: "29.97", FPS: 29.97}, {Name: "30", FPS: 30}, {Name: "48", FPS: 48},
		{Name: "50", FPS: 50}, {Name: "59.94", FPS: 59.94}, {Name: "60", FPS: 60},
		{Name: "120", FPS: 120}, {Name: "240", FPS: 240},
	},
	VideoBitrates: []int{500_000, 1_000_000, 1_500_000, 2_000_000, 3_000_000, 4_000_000,
		5_000_000, 6_000_000, 8_000_000, 10_000_000, 12_000_000, 15_000_000, 20_000_000,
		25_000_000, 30_000_000, 40_000_000, 50_000_000, 80_000_000, 100_000_000},
	AudioBitrates: []int{32_000, 48_000, 64_000, 96_000, 128_000, 160_000, 192_000, 224_000,
		256_000, 320_000, 384_000, 448_000, 512_000, 640_000},
	SampleRates: []int{8000, 11025, 12000, 16000, 22050, 24000, 32000, 44100, 48000, 64000, 88200,
		96000, 176400, 192000},
	Channels: []ChannelLayout{
		{Count: 1, Name: "Mono"}, {Count: 2, Name: "Stereo"}, {Count: 6, Name: "5.1"},
		{Count: 8, Name: "7.1"},
	},
}

// Options returns the value lists the settings dialog offers. The result is a copy the caller may
// modify.
func Options() Domains {
	d := domains
	d.Resolutions = slices.Clone(d.Resolutions)
	d.FrameRates = slices.Clone(d.FrameRates)
	d.VideoBitrates = slices.Clone(d.VideoBitrates)
	d.AudioBitrates = slices.Clone(d.AudioBitrates)
	d.SampleRates = slices.Clone(d.SampleRates)
	d.Channels = slices.Clone(d.Channels)
	return d
}

// RateArg returns the value to pass to ffmpeg's -r for fps, spelling the NTSC rates as exact
// fractions so 29.97 becomes 30000/1001.
func RateArg(fps float64) string {
	for _, base := range []int{24, 30, 60, 120, 240} {
		if SameRate(fps, float64(base)*1000/1001) {
			return strconv.Itoa(base*1000) + "/1001"
		}
	}
	return strconv.FormatFloat(fps, 'f', -1, 64)
}
