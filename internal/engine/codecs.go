package engine

import (
	"slices"
	"strconv"
	"strings"

	"github.com/dcadolph/icelos/internal/format"
)

// videoSpec is what a video encoder's argument builder works from.
type videoSpec struct {
	// Video is the stream settings, with any mode bitrate already filled in.
	Video format.Video
	// Bitrate is the video bitrate in bits per second, from the settings or a target size.
	Bitrate int
	// Pass is the pass number of a two-pass encode, zero for a single pass.
	Pass int
	// PassLog is the statistics file prefix of a two-pass encode.
	PassLog string
}

// videoArgsFunc returns an encoder's profile, level, rate control, and tuning arguments.
type videoArgsFunc func(s videoSpec) []string

// videoEncoder is how icelos drives one ffmpeg video encoder.
type videoEncoder struct {
	// Setup holds the arguments that follow -c:v, such as the speed preset.
	Setup []string
	// PixFmts maps a catalog pixel format to the name this encoder takes, for encoders that
	// read the same layout under another name.
	PixFmts map[string]string
	// Args returns the encoder arguments that follow the frame settings.
	Args videoArgsFunc
	// Accepts reports whether the encoder can honor the settings. Nil accepts everything.
	Accepts func(v format.Video) bool
	// RefusedBy lists the muxers that cannot carry this encoder's output.
	RefusedBy []string
	// Trial is true for encoders that count as available only after a trial encode passes,
	// because ffmpeg lists them whether or not the machine has the GPU they run on.
	Trial bool
}

// audioArgsFunc returns an audio encoder's own arguments for the settings, given the output
// video frame rate, which is zero when there is no video or it follows the source.
type audioArgsFunc func(a format.Audio, fps float64) []string

// audioEncoder is how icelos drives one ffmpeg audio encoder.
type audioEncoder struct {
	// Args returns the encoder arguments that follow the generic audio settings. Nil adds none.
	Args audioArgsFunc
	// Accepts reports whether the encoder can honor the settings. Nil accepts everything.
	Accepts func(a format.Audio) bool
}

// Quality scales map format.Quality to each encoder's own scale, highest quality first.
var ( //nolint:gochecknoglobals // Read-only tables.
	x264Quality   = [3]int{20, 23, 27}
	x265Quality   = [3]int{22, 26, 29}
	vtH264Quality = [3]int{70, 60, 48}
	vtHEVCQuality = [3]int{65, 55, 42}
	svtQuality    = [3]int{27, 33, 40}
	vp9Quality    = [3]int{28, 33, 40}
	vp8Quality    = [3]int{6, 10, 18}
	qscaleQuality = [3]int{3, 5, 8}
	cfhdQuality   = [3]int{3, 6, 10}
	theoraQuality = [3]int{8, 6, 4}
	// The GPU encoder scales follow the software encoder of the same codec on each encoder's
	// own range, as ffmpeg 9 defines it. NVENC -cq, Quick Sync ICQ, and AMF constant QP read
	// H.264 and HEVC on the 0 to 51 quantizer range of x264 and x265, so those tiers carry over.
	// For AV1, NVENC -cq runs 0 to 63 like the SVT-AV1 CRF, AMF QP runs 0 to 255 as AV1's
	// qindex at four steps per CRF step, and ffmpeg clamps Quick Sync ICQ to 1 to 51 for every
	// codec, so the SVT-AV1 tiers are scaled by 51/63.
	amfAV1Quality = [3]int{108, 132, 160}
	qsvAV1Quality = [3]int{22, 27, 32}
)

// Encoder settings that are not user choices.
const (
	// softwarePreset is the x264 and x265 speed preset.
	softwarePreset = "medium"
	// svtPreset is the SVT-AV1 speed preset, where higher is faster.
	svtPreset = "8"
	// vpxCPUUsed is the libvpx speed setting, where higher is faster.
	vpxCPUUsed = "3"
	// vp8MaxBitrate caps VP8's constrained quality mode, which needs a bitrate ceiling.
	vp8MaxBitrate = "30M"
	// amvSampleRate is the only sample rate AMV audio carries.
	amvSampleRate = 22050
	// nvencPreset is the NVENC speed preset, from p1 fastest to p7 slowest.
	nvencPreset = "p5"
	// qsvPreset is the Quick Sync speed preset.
	qsvPreset = "medium"
	// amfPreset is the AMF quality preset.
	amfPreset = "quality"
)

// vtH264Levels are the H.264 levels h264_videotoolbox can signal.
var vtH264Levels = []string{ //nolint:gochecknoglobals // Read-only table.
	"3.0", "3.1", "3.2", "4.0", "4.1", "4.2", "5.0", "5.1", "5.2"}

// GPU encoder settings shared by a family. Quick Sync takes NV12 and P010 frames only, and
// none of the three families reads planar 10-bit 4:2:0, so 10-bit goes in as P010.
var ( //nolint:gochecknoglobals // Read-only tables.
	nvencSetup  = []string{"-preset", nvencPreset, "-tune", "hq"}
	qsvSetup    = []string{"-preset", qsvPreset}
	amfSetup    = []string{"-quality", amfPreset}
	gpuPixFmts  = map[string]string{"yuv420p10le": "p010le"}
	qsvPixFmts  = map[string]string{"yuv420p": "nv12", "yuv420p10le": "p010le"}
	amfProfiles = map[string]string{"baseline": "constrained_baseline"}
)

// videoEncoders holds every catalog video encoder the bundled ffmpeg build offers. Encoders the
// catalog lists but this table lacks are reported missing by Build.
var videoEncoders = map[string]videoEncoder{ //nolint:gochecknoglobals // Read-only table.
	"libx264": {Setup: []string{"-preset", softwarePreset}, Args: func(s videoSpec) []string {
		args := profileLevel(s.Video, nil)
		if s.Video.RateControl == format.RateQuality {
			return append(args, "-crf", scaled(x264Quality, s.Video.Quality))
		}
		return append(args, bitrateArgs(s)...)
	}},
	"libx265": {Setup: []string{"-preset", softwarePreset}, Args: x265Args},
	"h264_videotoolbox": {Args: videoToolboxArgs(vtH264Quality, nil),
		Accepts: func(v format.Video) bool {
			return v.Level == "" || slices.Contains(vtH264Levels, v.Level)
		}},
	"hevc_videotoolbox": {Args: videoToolboxArgs(vtHEVCQuality, nil),
		PixFmts: map[string]string{"yuv420p10le": "p010le"},
		Accepts: func(v format.Video) bool { return v.Level == "" }},
	"h264_nvenc": {Setup: nvencSetup, Args: nvencArgs(x264Quality), PixFmts: gpuPixFmts,
		Trial: true},
	"hevc_nvenc": {Setup: nvencSetup, Args: nvencArgs(x265Quality), PixFmts: gpuPixFmts,
		Trial: true},
	"av1_nvenc": {Setup: nvencSetup, Args: nvencArgs(svtQuality), PixFmts: gpuPixFmts,
		Trial: true},
	"h264_qsv": {Setup: qsvSetup, Args: qsvArgs(x264Quality), PixFmts: qsvPixFmts,
		Accepts: noLevel, Trial: true},
	"hevc_qsv": {Setup: qsvSetup, Args: qsvArgs(x265Quality), PixFmts: qsvPixFmts,
		Accepts: noLevel, Trial: true},
	"av1_qsv": {Setup: qsvSetup, Args: qsvArgs(qsvAV1Quality), PixFmts: qsvPixFmts,
		Accepts: noLevel, Trial: true},
	"h264_amf": {Setup: amfSetup, Args: amfArgs(x264Quality, true), PixFmts: gpuPixFmts,
		Trial: true},
	"hevc_amf": {Setup: amfSetup, Args: amfArgs(x265Quality, false), PixFmts: gpuPixFmts,
		Trial: true},
	"av1_amf": {Setup: amfSetup, Args: amfArgs(amfAV1Quality, true), PixFmts: gpuPixFmts,
		Trial: true},
	"libsvtav1": {Setup: []string{"-preset", svtPreset}, Args: func(s videoSpec) []string {
		if s.Video.RateControl == format.RateQuality {
			return []string{"-crf", scaled(svtQuality, s.Video.Quality)}
		}
		return bitrateArgs(s)
	}},
	"libvpx-vp9": {Setup: []string{"-deadline", "good", "-cpu-used", vpxCPUUsed, "-row-mt", "1"},
		Args: func(s videoSpec) []string {
			if s.Video.RateControl == format.RateQuality {
				return []string{"-crf", scaled(vp9Quality, s.Video.Quality), "-b:v", "0"}
			}
			return bitrateArgs(s)
		}},
	"libvpx": {Setup: []string{"-deadline", "good", "-cpu-used", vpxCPUUsed},
		Args: func(s videoSpec) []string {
			if s.Video.RateControl == format.RateQuality {
				return []string{"-crf", scaled(vp8Quality, s.Video.Quality), "-b:v", vp8MaxBitrate}
			}
			return bitrateArgs(s)
		}},
	"mpeg4":      {Args: qscaleArgs},
	"mjpeg":      {Args: qscaleArgs},
	"msmpeg4":    {Args: bitrateArgs},
	"wmv1":       {Args: bitrateArgs},
	"wmv2":       {Args: bitrateArgs},
	"flv":        {Args: bitrateArgs},
	"h263":       {Args: bitrateArgs},
	"mpeg1video": {Args: bitrateArgs},
	"mpeg2video": {Args: bitrateArgs},
	"amv":        {Args: bitrateArgs},
	"dvvideo": {Args: func(videoSpec) []string {
		return []string{"-chroma_sample_location", "topleft"}
	}},
	"libtheora": {Args: func(s videoSpec) []string {
		if s.Video.RateControl == format.RateQuality {
			return []string{"-q:v", scaled(theoraQuality, s.Video.Quality)}
		}
		return bitrateArgs(s)
	}},
	"ffv1": {Args: func(videoSpec) []string { return []string{"-level", "3"} }},
	"prores_ks": {Args: func(s videoSpec) []string {
		return append(profileLevel(s.Video, nil), "-vendor", "apl0")
	}},
	"prores_videotoolbox": {Args: videoToolboxArgs([3]int{}, map[string]string{"4444xq": "xq"}),
		PixFmts:   map[string]string{"yuv422p10le": "p210le", "yuv444p10le": "p416le"},
		RefusedBy: []string{"mxf"}},
	"dnxhd": {Args: func(s videoSpec) []string {
		args := profileLevel(s.Video, nil)
		if s.Bitrate > 0 {
			args = append(args, "-b:v", strconv.Itoa(s.Bitrate))
		}
		return args
	}},
	"cfhd": {Args: func(s videoSpec) []string {
		return []string{"-quality", scaled(cfhdQuality, s.Video.Quality)}
	}},
}

// audioEncoders holds every catalog audio encoder the bundled ffmpeg build offers. The generic
// bitrate, channel, and sample rate arguments come from Build; these add the rest.
var audioEncoders = map[string]audioEncoder{ //nolint:gochecknoglobals // Read-only table.
	"aac_at":            {},
	"aac":               {},
	"libmp3lame":        {},
	"ac3":               {},
	"eac3":              {},
	"libopus":           {},
	"libvorbis":         {},
	"wmav2":             {},
	"mp2":               {},
	"libopencore_amrnb": {},
	"pcm_s16le":         {},
	"pcm_s24le":         {},
	"pcm_s16be":         {},
	"pcm_s24be":         {},
	"flac": {Args: func(a format.Audio, _ float64) []string {
		switch a.BitDepth {
		case 16:
			return []string{"-sample_fmt", "s16"}
		case 24:
			return []string{"-sample_fmt", "s32", "-bits_per_raw_sample", "24"}
		}
		return nil
	}},
	"alac": {Args: func(a format.Audio, _ float64) []string {
		switch a.BitDepth {
		case 16:
			return []string{"-sample_fmt", "s16p"}
		case 24:
			return []string{"-sample_fmt", "s32p"}
		}
		return nil
	}},
	"alac_at": {Accepts: func(a format.Audio) bool { return a.BitDepth == 0 || a.BitDepth == 16 }},
	"adpcm_ima_amv": {Args: func(_ format.Audio, fps float64) []string {
		if fps <= 0 {
			return nil
		}
		return []string{"-block_size", strconv.Itoa(int(amvSampleRate / fps))}
	}},
}

// subtitleEncoders lists the catalog subtitle encoders, which take no arguments of their own.
var subtitleEncoders = []string{"mov_text", "srt", "ass", "webvtt"} //nolint:gochecknoglobals // Read-only table.

// scaled returns the entry of scale for quality q as an argument.
func scaled(scale [3]int, q format.Quality) string {
	return strconv.Itoa(scale[qualityIndex(q)])
}

// profileLevel returns -profile:v and -level:v for the settings, renaming profiles through names
// for encoders that spell them differently.
func profileLevel(v format.Video, names map[string]string) []string {
	var args []string
	if v.Profile != "" {
		profile := v.Profile
		if name, ok := names[profile]; ok {
			profile = name
		}
		args = append(args, "-profile:v", profile)
	}
	if v.Level != "" {
		args = append(args, "-level:v", v.Level)
	}
	return args
}

// bitrateArgs returns ffmpeg's generic bitrate arguments for the rate control, including the
// statistics arguments of a two-pass encode.
func bitrateArgs(s videoSpec) []string {
	b := strconv.Itoa(s.Bitrate)
	switch s.Video.RateControl {
	case format.RateCBR:
		return []string{"-b:v", b, "-minrate", b, "-maxrate", b, "-bufsize", strconv.Itoa(2 * s.Bitrate)}
	case format.RateTwoPass:
		return []string{"-b:v", b, "-pass", strconv.Itoa(s.Pass), "-passlogfile", s.PassLog}
	}
	return []string{"-b:v", b}
}

// qscaleArgs returns -q:v for the quality rate control and the generic bitrate arguments
// otherwise, for the MPEG-4 Part 2 and Motion JPEG encoders.
func qscaleArgs(s videoSpec) []string {
	if s.Video.RateControl == format.RateQuality {
		return []string{"-q:v", scaled(qscaleQuality, s.Video.Quality)}
	}
	return bitrateArgs(s)
}

// x265Args returns libx265's arguments. Level and two-pass statistics go through -x265-params,
// which also quiets x265's own logging.
func x265Args(s videoSpec) []string {
	v := s.Video
	var args, params []string
	if v.Profile != "" {
		args = append(args, "-profile:v", v.Profile)
	}
	switch v.RateControl {
	case format.RateQuality:
		args = append(args, "-crf", scaled(x265Quality, v.Quality))
	case format.RateTwoPass:
		args = append(args, "-b:v", strconv.Itoa(s.Bitrate))
		params = append(params, "pass="+strconv.Itoa(s.Pass), "stats="+s.PassLog)
	default:
		args = append(args, bitrateArgs(s)...)
	}
	if v.Level != "" {
		params = append(params, "level-idc="+v.Level)
	}
	params = append(params, "log-level=error")
	return append(args, "-x265-params", strings.Join(params, ":"))
}

// videoToolboxArgs returns the argument builder for a VideoToolbox encoder with the given quality
// scale and profile names. VideoToolbox has no two-pass mode, so Build never picks it for one.
func videoToolboxArgs(scale [3]int, names map[string]string) videoArgsFunc {
	return func(s videoSpec) []string {
		args := profileLevel(s.Video, names)
		switch s.Video.RateControl {
		case format.RateQuality:
			args = append(args, "-q:v", scaled(scale, s.Video.Quality))
		case format.RateCBR:
			args = append(args, "-b:v", strconv.Itoa(s.Bitrate), "-constant_bit_rate", "1")
		case format.RateVBR:
			args = append(args, "-b:v", strconv.Itoa(s.Bitrate))
		}
		return args
	}
}

// noLevel accepts settings that leave the level to the encoder, for encoders that cannot take
// a level by name.
func noLevel(v format.Video) bool {
	return v.Level == ""
}

// peakRate returns the bitrate ceiling of a GPU encoder's variable bitrate mode, twice the
// average.
func peakRate(bitrate int) string {
	return strconv.Itoa(2 * bitrate)
}

// nvencArgs returns the argument builder for an NVENC encoder with the given -cq scale.
// Constant quality is NVENC's VBR mode with a quality target, where ffmpeg drops the average
// bitrate and honors only a ceiling.
func nvencArgs(scale [3]int) videoArgsFunc {
	return func(s videoSpec) []string {
		args := profileLevel(s.Video, nil)
		switch s.Video.RateControl {
		case format.RateQuality:
			return append(args, "-rc", "vbr", "-cq", scaled(scale, s.Video.Quality))
		case format.RateCBR:
			return append(args, "-rc", "cbr", "-b:v", strconv.Itoa(s.Bitrate))
		}
		return append(args, "-rc", "vbr", "-b:v", strconv.Itoa(s.Bitrate), "-maxrate",
			peakRate(s.Bitrate))
	}
}

// qsvArgs returns the argument builder for a Quick Sync encoder with the given quality scale.
// ffmpeg picks the Quick Sync rate control from the arguments: a global quality with no
// ceiling is ICQ, and a ceiling equal to the bitrate is CBR.
func qsvArgs(scale [3]int) videoArgsFunc {
	return func(s videoSpec) []string {
		args := profileLevel(s.Video, nil)
		b := strconv.Itoa(s.Bitrate)
		switch s.Video.RateControl {
		case format.RateQuality:
			return append(args, "-global_quality", scaled(scale, s.Video.Quality))
		case format.RateCBR:
			return append(args, "-b:v", b, "-maxrate", b)
		}
		return append(args, "-b:v", b, "-maxrate", peakRate(s.Bitrate))
	}
}

// amfArgs returns the argument builder for an AMF encoder with the given QP scale. Quality is
// constant QP on every frame type the encoder takes a QP for, and bFrames says whether it
// takes one for B-frames.
func amfArgs(scale [3]int, bFrames bool) videoArgsFunc {
	return func(s videoSpec) []string {
		args := profileLevel(s.Video, amfProfiles)
		switch s.Video.RateControl {
		case format.RateQuality:
			q := scaled(scale, s.Video.Quality)
			args = append(args, "-rc", "cqp", "-qp_i", q, "-qp_p", q)
			if bFrames {
				args = append(args, "-qp_b", q)
			}
			return args
		case format.RateCBR:
			return append(args, "-rc", "cbr", "-b:v", strconv.Itoa(s.Bitrate))
		}
		return append(args, "-rc", "vbr_peak", "-b:v", strconv.Itoa(s.Bitrate), "-maxrate",
			peakRate(s.Bitrate))
	}
}
