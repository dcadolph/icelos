// Package probe reads media file metadata through ffprobe.
package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Stream describes one stream inside a media file.
type Stream struct {
	// Index is the stream position in the container.
	Index int `json:"index"`
	// Kind is "video", "audio", or "subtitle".
	Kind string `json:"codec_type"`
	// Codec is the codec short name, e.g. "h264".
	Codec string `json:"codec_name"`
	// Width is the frame width in pixels for video streams.
	Width int `json:"width"`
	// Height is the frame height in pixels for video streams.
	Height int `json:"height"`
	// Channels is the channel count for audio streams.
	Channels int `json:"channels"`
	// SampleRate is the sample rate in hertz for audio streams, zero when unknown.
	SampleRate int `json:"sample_rate,string,omitempty"`
	// AvgFrameRate is the average frame rate of a video stream as a fraction, such as "30000/1001".
	AvgFrameRate string `json:"avg_frame_rate,omitempty"`
	// BaseFrameRate is the lowest frame rate that represents every timestamp, as a fraction.
	BaseFrameRate string `json:"r_frame_rate,omitempty"`
	// Disposition carries the stream role flags.
	Disposition Disposition `json:"disposition"`
	// SideData carries per-stream extras such as the display matrix.
	SideData []SideData `json:"side_data_list"`
}

// Disposition holds the ffprobe role flags for a stream.
type Disposition struct {
	// AttachedPic is 1 when the stream is embedded cover art rather than motion video.
	AttachedPic int `json:"attached_pic"`
}

// SideData is one ffprobe side data entry.
type SideData struct {
	// Rotation is the display rotation in degrees from a display matrix entry.
	Rotation int `json:"rotation"`
}

// IsCover reports whether the stream is embedded cover art.
func (s Stream) IsCover() bool {
	return s.Disposition.AttachedPic == 1
}

// DisplaySize returns the frame size as shown to the viewer, swapping width and height when
// the stream carries a quarter-turn rotation.
func (s Stream) DisplaySize() (int, int) {
	for _, sd := range s.SideData {
		if r := ((sd.Rotation % 360) + 360) % 360; r == 90 || r == 270 {
			return s.Height, s.Width
		}
	}
	return s.Width, s.Height
}

// FrameRate returns the average frame rate in frames per second, falling back to the base rate,
// or zero when ffprobe reported neither.
func (s Stream) FrameRate() float64 {
	for _, r := range []string{s.AvgFrameRate, s.BaseFrameRate} {
		if fps := parseRate(r); fps > 0 {
			return fps
		}
	}
	return 0
}

// parseRate parses a frame rate fraction such as "30000/1001" or a plain number, returning zero
// for anything else.
func parseRate(r string) float64 {
	num, den, found := strings.Cut(r, "/")
	n, err := strconv.ParseFloat(num, 64)
	if err != nil || n <= 0 {
		return 0
	}
	if !found {
		return n
	}
	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d <= 0 {
		return 0
	}
	return n / d
}

// FirstVideo returns the first motion video stream.
func (i Info) FirstVideo() (Stream, bool) {
	for _, s := range i.Streams {
		if s.Kind == "video" && !s.IsCover() {
			return s, true
		}
	}
	return Stream{}, false
}

// FirstAudio returns the first audio stream.
func (i Info) FirstAudio() (Stream, bool) {
	for _, s := range i.Streams {
		if s.Kind == "audio" {
			return s, true
		}
	}
	return Stream{}, false
}

// Info holds the metadata for one media file.
type Info struct {
	// Format is the container short name list, e.g. "mov,mp4,m4a,3gp,3g2,mj2".
	Format string `json:"format"`
	// Duration is the total playing time.
	Duration time.Duration `json:"durationNs"`
	// Size is the file size in bytes.
	Size int64 `json:"size"`
	// Streams lists every stream in the file.
	Streams []Stream `json:"streams"`
}

// rawOutput mirrors the ffprobe JSON document.
type rawOutput struct {
	// Format is the container section.
	Format struct {
		// Name is the container format name.
		Name string `json:"format_name"`
		// Duration is the playing time in seconds as a decimal string.
		Duration string `json:"duration"`
		// Size is the file size in bytes as a decimal string.
		Size string `json:"size"`
	} `json:"format"`
	// Streams is the stream section.
	Streams []Stream `json:"streams"`
}

// Parse decodes ffprobe JSON output into Info.
func Parse(data []byte) (Info, error) {
	var raw rawOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return Info{}, fmt.Errorf("%w: decode: %w", ErrProbe, err)
	}
	info := Info{Format: raw.Format.Name, Streams: raw.Streams}
	if raw.Format.Duration != "" {
		secs, err := strconv.ParseFloat(raw.Format.Duration, 64)
		if err != nil {
			return Info{}, fmt.Errorf("%w: duration: %w", ErrProbe, err)
		}
		info.Duration = time.Duration(secs * float64(time.Second))
	}
	if raw.Format.Size != "" {
		size, err := strconv.ParseInt(raw.Format.Size, 10, 64)
		if err != nil {
			return Info{}, fmt.Errorf("%w: size: %w", ErrProbe, err)
		}
		info.Size = size
	}
	return info, nil
}

// File runs the ffprobe found on PATH against path and returns the parsed metadata.
func File(ctx context.Context, path string) (Info, error) {
	return Run(ctx, "ffprobe", path)
}

// Run runs the ffprobe binary at bin against path and returns the parsed metadata.
func Run(ctx context.Context, bin, path string) (Info, error) {
	cmd := exec.CommandContext(ctx, bin, "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", "-i", path)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return Info{}, fmt.Errorf("%w: %s: %s", ErrProbe, path, bytes.TrimSpace(exitErr.Stderr))
		}
		return Info{}, fmt.Errorf("%w: %s: %w", ErrProbe, path, err)
	}
	return Parse(out)
}
