// Package probe reads media file metadata through ffprobe.
package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dcadolph/icelos/internal/util/proc"
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
	// BFrames is how many frames the decoder holds back to reorder B-frames, zero without them.
	BFrames int `json:"has_b_frames,omitempty"`
	// Bitrate is the stream's bits per second where the container records it, zero otherwise.
	Bitrate int `json:"bit_rate,string,omitempty"`
	// Duration is the stream's playing time in seconds as a decimal string, empty when the
	// container records none, as Matroska does.
	Duration string `json:"duration,omitempty"`
	// Disposition carries the stream role flags.
	Disposition Disposition `json:"disposition"`
	// Tags carries the stream's language and title.
	Tags Tags `json:"tags,omitzero"`
	// SideData carries per-stream extras such as the display matrix.
	SideData []SideData `json:"side_data_list"`
}

// Disposition holds the ffprobe role flags for a stream.
type Disposition struct {
	// AttachedPic is 1 when the stream is embedded cover art rather than motion video.
	AttachedPic int `json:"attached_pic"`
}

// Tags holds the stream tags icelos reads: the language and title shown when choosing tracks,
// and the duration Matroska records in place of a stream duration.
type Tags struct {
	// Language is the ISO 639-2 language code, such as "eng", empty when untagged.
	Language string `json:"language,omitempty"`
	// Title is the track name the file gives, empty when untagged.
	Title string `json:"title,omitempty"`
	// Duration is the stream's playing time as hours, minutes, and seconds, such as
	// "00:00:02.028000000", empty when untagged.
	Duration string `json:"DURATION,omitempty"`
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

// playTime returns the stream's playing time from its duration or its Matroska duration tag,
// and zero when it records neither in a form that parses.
func (s Stream) playTime() time.Duration {
	if d, err := seconds(s.Duration); err == nil && d > 0 {
		return d
	}
	return clock(s.Tags.Duration)
}

// clock parses an hours, minutes, and seconds time such as "00:00:02.028000000", returning zero
// for anything else.
func clock(s string) time.Duration {
	h, rest, ok := strings.Cut(s, ":")
	if !ok {
		return 0
	}
	m, sec, ok := strings.Cut(rest, ":")
	if !ok {
		return 0
	}
	hours, herr := strconv.Atoi(h)
	mins, merr := strconv.Atoi(m)
	secs, serr := strconv.ParseFloat(sec, 64)
	if herr != nil || merr != nil || serr != nil || hours < 0 || mins < 0 || secs < 0 {
		return 0
	}
	return time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute +
		time.Duration(math.Round(secs*float64(time.Second)))
}

// MediaDuration returns the longest playing time among the motion video and audio streams, the
// span a bitrate spreads over. A subtitle track can run past the picture and sound and stretch
// the container's duration, so the container value is used only when no video or audio stream
// records one.
func (i Info) MediaDuration() time.Duration {
	var longest time.Duration
	for _, s := range i.Streams {
		if (s.Kind == "video" && !s.IsCover()) || s.Kind == "audio" {
			longest = max(longest, s.playTime())
		}
	}
	if longest > 0 {
		return longest
	}
	return i.Duration
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

// Kind returns the streams of the given kind in file order, so position n is the stream ffmpeg
// selects with the specifier "a:n" or "s:n".
func (i Info) Kind(kind string) []Stream {
	var out []Stream
	for _, s := range i.Streams {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

// Chapter is one chapter mark in a media file.
type Chapter struct {
	// Start is where the chapter begins.
	Start time.Duration `json:"startNs"`
	// End is where the chapter ends.
	End time.Duration `json:"endNs"`
	// Title is the chapter name, empty when untitled.
	Title string `json:"title,omitempty"`
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
	// Chapters lists the chapter marks in playing order.
	Chapters []Chapter `json:"chapters,omitempty"`
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
	// Chapters is the chapter section.
	Chapters []rawChapter `json:"chapters"`
}

// rawChapter mirrors one ffprobe chapter entry.
type rawChapter struct {
	// StartTime is the start in seconds as a decimal string.
	StartTime string `json:"start_time"`
	// EndTime is the end in seconds as a decimal string.
	EndTime string `json:"end_time"`
	// Tags carries the chapter title.
	Tags struct {
		// Title is the chapter name.
		Title string `json:"title"`
	} `json:"tags"`
}

// Parse decodes ffprobe JSON output into Info.
func Parse(data []byte) (Info, error) {
	var raw rawOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return Info{}, fmt.Errorf("%w: decode: %w", ErrProbe, err)
	}
	info := Info{Format: raw.Format.Name, Streams: raw.Streams}
	if raw.Format.Duration != "" {
		d, err := seconds(raw.Format.Duration)
		if err != nil {
			return Info{}, fmt.Errorf("%w: duration: %w", ErrProbe, err)
		}
		info.Duration = d
	}
	for _, c := range raw.Chapters {
		start, err := seconds(c.StartTime)
		if err != nil {
			return Info{}, fmt.Errorf("%w: chapter start: %w", ErrProbe, err)
		}
		end, err := seconds(c.EndTime)
		if err != nil {
			return Info{}, fmt.Errorf("%w: chapter end: %w", ErrProbe, err)
		}
		info.Chapters = append(info.Chapters, Chapter{Start: start, End: end, Title: c.Tags.Title})
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

// seconds parses a decimal seconds string such as "12.5" into a duration.
func seconds(s string) (time.Duration, error) {
	secs, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// File runs the ffprobe found on PATH against path and returns the parsed metadata.
func File(ctx context.Context, path string) (Info, error) {
	return Run(ctx, "ffprobe", path)
}

// Run runs the ffprobe binary at bin against path and returns the parsed metadata.
func Run(ctx context.Context, bin, path string) (Info, error) {
	cmd := proc.Command(ctx, bin, "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", "-show_chapters", "-i", path)
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
