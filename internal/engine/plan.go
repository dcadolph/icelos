package engine

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// Plan is the ffmpeg work for one job.
type Plan struct {
	// Passes holds the ffmpeg arguments for each pass in run order.
	Passes [][]string
	// Encoder is the video encoder name, or empty when video is copied or absent.
	Encoder string
	// Hardware is true when Encoder is a VideoToolbox encoder.
	Hardware bool
}

// codecEncoders names the software and hardware encoder for one codec.
type codecEncoders struct {
	// Software is the CPU encoder name.
	Software string
	// Hardware is the VideoToolbox encoder name.
	Hardware string
}

// Encoder tables keyed by catalog codec ID. Quality values are indexed by format.Quality
// minus one.
var (
	videoEncoders = map[string]codecEncoders{ //nolint:gochecknoglobals // Read-only table.
		"h264": {Software: "libx264", Hardware: "h264_videotoolbox"},
		"hevc": {Software: "libx265", Hardware: "hevc_videotoolbox"},
	}
	qualityScale = map[string][3]int{ //nolint:gochecknoglobals // Read-only table.
		"libx264":           {20, 23, 27},
		"libx265":           {22, 26, 29},
		"h264_videotoolbox": {70, 60, 48},
		"hevc_videotoolbox": {65, 55, 42},
	}
	movFamily = []string{"mp4", "m4v", "m4a", "mov"} //nolint:gochecknoglobals // Read-only table.
)

// Bitrate budget for target-size encodes.
const (
	// muxOverhead is the share of the target left after container overhead.
	muxOverhead = 0.97
	// minVideoBitrate is the lowest video bitrate worth producing, in bits per second.
	minVideoBitrate = 64_000
)

// TargetVideoBitrate returns the video bitrate in bits per second that lands a file of
// duration dur at target bytes, after audioBps of audio and container overhead.
func TargetVideoBitrate(target int64, dur time.Duration, audioBps int) (int, error) {
	if dur <= 0 {
		return 0, ErrNoDuration
	}
	total := float64(target) * 8 * muxOverhead / dur.Seconds()
	video := total - float64(audioBps)
	if video < minVideoBitrate {
		return 0, fmt.Errorf("%w: %d bytes over %s leaves %.0f bps for video", ErrTargetTooSmall,
			target, dur.Round(time.Second), max(video, 0))
	}
	return int(video), nil
}

// Build plans the ffmpeg passes that turn job.Input into dest. Two-pass encodes write their
// statistics under passlog. When job.Hardware is set and avail offers a VideoToolbox encoder
// for the codec, the plan uses it, except for target-size encodes, which need two passes.
func Build(job Job, info probe.Info, avail EncoderSet, dest, passlog string) (Plan, error) {
	p := job.Preset
	if err := p.Validate(); err != nil {
		return Plan{}, err
	}
	video, hasVideo := info.FirstVideo()
	audio, hasAudio := info.FirstAudio()
	if p.HasVideo() && !hasVideo {
		return Plan{}, fmt.Errorf("%w: %s: pick an audio preset", ErrNoVideo, job.Input)
	}
	if !p.HasVideo() && !hasAudio {
		return Plan{}, fmt.Errorf("%w: %s", ErrNoAudio, job.Input)
	}
	useAudio := p.HasAudio() && hasAudio

	head := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-nostats",
		"-progress", "pipe:1", "-y", "-i", job.Input}
	var mapVideo, mapAudio []string
	if p.HasVideo() {
		mapVideo = []string{"-map", "0:" + strconv.Itoa(video.Index)}
	}
	if useAudio {
		mapAudio = []string{"-map", "0:" + strconv.Itoa(audio.Index)}
	}
	var audioArgs []string
	if useAudio {
		var err error
		if audioArgs, err = audioCodecArgs(p.Audio, audio, avail); err != nil {
			return Plan{}, err
		}
	}
	tail := []string{"-map_metadata", "0"}
	if slices.Contains(movFamily, p.Ext()) {
		tail = append(tail, "-movflags", "+faststart")
	}
	tail = append(tail, dest)

	if !p.HasVideo() {
		return Plan{Passes: [][]string{slices.Concat(head, mapAudio, audioArgs, tail)}}, nil
	}
	if p.Video.Codec == format.Copy {
		vargs := []string{"-c:v", "copy"}
		if video.Codec == "hevc" && slices.Contains(movFamily, p.Ext()) {
			vargs = append(vargs, "-tag:v", "hvc1")
		}
		return Plan{Passes: [][]string{slices.Concat(head, mapVideo, vargs, mapAudio, audioArgs, tail)}}, nil
	}

	names := videoEncoders[p.Video.Codec]
	enc, hw := names.Software, false
	if job.Hardware && p.TargetBytes == 0 && avail.Has(names.Hardware) {
		enc, hw = names.Hardware, true
	}
	if !avail.Has(enc) {
		return Plan{}, fmt.Errorf("%w: %s", ErrMissingEncoder, enc)
	}
	vargs := []string{"-c:v", enc}
	if !hw {
		vargs = append(vargs, "-preset", "medium")
	}
	w, h := video.DisplaySize()
	if nw, nh, ok := fit(w, h, p.Video.MaxWidth, p.Video.MaxHeight); ok {
		vargs = append(vargs, "-vf", fmt.Sprintf("scale=%d:%d", nw, nh))
	}
	vargs = append(vargs, "-pix_fmt", "yuv420p")
	if p.Video.Codec == "hevc" && slices.Contains(movFamily, p.Ext()) {
		vargs = append(vargs, "-tag:v", "hvc1")
	}
	plan := Plan{Encoder: enc, Hardware: hw}

	if p.TargetBytes == 0 {
		q := qualityScale[enc][qualityIndex(p.Video.Quality)]
		if hw {
			vargs = append(vargs, "-q:v", strconv.Itoa(q))
		} else {
			vargs = append(vargs, "-crf", strconv.Itoa(q))
		}
		if enc == "libx265" {
			vargs = append(vargs, "-x265-params", "log-level=error")
		}
		plan.Passes = [][]string{slices.Concat(head, mapVideo, vargs, mapAudio, audioArgs, tail)}
		return plan, nil
	}

	audioBps := 0
	if useAudio {
		audioBps = p.Audio.Bitrate
	}
	bps, err := TargetVideoBitrate(p.TargetBytes, info.Duration, audioBps)
	if err != nil {
		return Plan{}, err
	}
	vargs = append(vargs, "-b:v", strconv.Itoa(bps))
	pass := func(n int) []string {
		if enc == "libx265" {
			return []string{"-x265-params", fmt.Sprintf("pass=%d:stats=%s:log-level=error", n, passlog)}
		}
		return []string{"-pass", strconv.Itoa(n), "-passlogfile", passlog}
	}
	first := slices.Concat(head, mapVideo, vargs, pass(1), []string{"-an", "-f", "null", os.DevNull})
	second := slices.Concat(head, mapVideo, vargs, pass(2), mapAudio, audioArgs, tail)
	plan.Passes = [][]string{first, second}
	return plan, nil
}

// audioCodecArgs returns the ffmpeg audio codec arguments for a and the source stream src.
func audioCodecArgs(a format.Audio, src probe.Stream, avail EncoderSet) ([]string, error) {
	var args []string
	switch a.Codec {
	case format.Copy:
		return []string{"-c:a", "copy"}, nil
	case "aac":
		name := "aac"
		if avail.Has("aac_at") {
			name = "aac_at"
		}
		args = []string{"-c:a", name}
	case "mp3":
		if !avail.Has("libmp3lame") {
			return nil, fmt.Errorf("%w: libmp3lame", ErrMissingEncoder)
		}
		args = []string{"-c:a", "libmp3lame"}
	default:
		return nil, fmt.Errorf("%w: audio codec %q", preset.ErrInvalid, a.Codec)
	}
	if a.Bitrate > 0 {
		args = append(args, "-b:a", strconv.Itoa(a.Bitrate))
	}
	if a.Channels > 0 && src.Channels > a.Channels {
		args = append(args, "-ac", strconv.Itoa(a.Channels))
	}
	return args, nil
}

// qualityIndex maps q to a quality table index, treating unset as balanced.
func qualityIndex(q format.Quality) int {
	if q < format.QualityHigh || q > format.QualitySmall {
		q = format.QualityBalanced
	}
	return int(q - format.QualityHigh)
}

// fit returns the even frame size that fits w by h inside the box, and whether it differs
// from the source. maxLong bounds the long side and maxShort the short side, so portrait
// video is treated the same as landscape. A zero bound is unbounded.
func fit(w, h, maxLong, maxShort int) (int, int, bool) {
	if w <= 0 || h <= 0 {
		return w, h, false
	}
	bound := func(v int) int {
		if v <= 0 {
			return math.MaxInt32
		}
		return v
	}
	boxW, boxH := bound(maxLong), bound(maxShort)
	if h > w {
		boxW, boxH = boxH, boxW
	}
	scale := min(1, float64(boxW)/float64(w), float64(boxH)/float64(h))
	nw := even(int(math.Round(float64(w) * scale)))
	nh := even(int(math.Round(float64(h) * scale)))
	return nw, nh, nw != w || nh != h
}

// even rounds n down to an even number of at least two.
func even(n int) int {
	return max(2, n-n%2)
}
