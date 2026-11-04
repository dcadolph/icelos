package engine

import (
	"math"
	"strings"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/probe"
)

// The estimate model, measured for the Mac app on 2026-11-03. The values are variables, not
// constants, so every expression rounds at run time the way the app's Double arithmetic does
// and both sides answer the same byte counts.
var ( //nolint:gochecknoglobals // Read-only model data.
	// estimateOverhead is the container's share on top of the streams.
	estimateOverhead = 1.01
	// assumedAudioBitrate stands in for a source audio stream, whose bitrate the probe omits.
	assumedAudioBitrate = 160_000.0
	// estimateSampleRate is the sample rate assumed for uncompressed audio when none is set.
	estimateSampleRate = 48_000.0
	// defaultAudioBitrate is the bitrate a lossy audio codec starts from when none is set.
	defaultAudioBitrate = 192_000.0
	// losslessShare is the size of FLAC and ALAC audio against the same audio uncompressed.
	losslessShare = 0.6
	// areaExponent bends the size change of a resize: half the pixels costs more than half the
	// bits.
	areaExponent = 0.75
	// spread widens a quality range by this share on each side for what the ratio model leaves
	// out: audio and container cost above the nominal bitrate and encoder variance.
	spread = 0.1
	// h264Ratios are the software H.264 ratios, which AV1 and VP9 scale until they are measured.
	h264Ratios = [3]ratio{{0.1128, 0.5134}, {0.0876, 0.1587}, {0.0523, 0.0650}}
	// softwareRatios bound output against source video bitrate for the quality rate control in
	// software, by codec ID and quality level less one. Each bound is the smaller or larger of a
	// clean and a grainy 20 second 1080p fixture.
	softwareRatios = map[string][3]ratio{
		"h264": h264Ratios,
		"hevc": {{0.0400, 0.1080}, {0.0173, 0.0701}, {0.0121, 0.0527}},
		"av1":  scaledRatios(h264Ratios, 0.7),
		"vp9":  scaledRatios(h264Ratios, 0.8),
	}
	// hardwareRatios are the same bounds measured on the Apple media engine.
	hardwareRatios = map[string][3]ratio{
		"h264": {{0.1741, 0.4687}, {0.0517, 0.1112}, {0.0257, 0.0909}},
		"hevc": {{0.0454, 0.1107}, {0.0232, 0.0777}, {0.0135, 0.0506}},
	}
)

// SizeRange is an expected output size in bytes. A point estimate has Low equal to High.
type SizeRange struct {
	// Low is the smallest expected size in bytes.
	Low int64 `json:"low"`
	// High is the largest expected size in bytes.
	High int64 `json:"high"`
}

// Point reports whether the estimate is one size.
func (r SizeRange) Point() bool {
	return r.Low == r.High
}

// Saving returns how much smaller the high bound is than source as a whole percent, negative
// when the output is larger, and false when either size is unknown.
func (r SizeRange) Saving(source int64) (int, bool) {
	if source <= 0 || r.High <= 0 {
		return 0, false
	}
	return int(math.Round((1 - float64(r.High)/float64(source)) * 100)), true
}

// ratio is the low and high share of the source video bitrate an encode keeps.
type ratio struct {
	// Low is the share on footage that compresses well.
	Low float64
	// High is the share on footage that does not.
	High float64
}

// scaled returns both bounds times factor.
func (r ratio) scaled(factor float64) ratio {
	return ratio{Low: r.Low * factor, High: r.High * factor}
}

// scaledRatios returns every ratio of rs times factor.
func scaledRatios(rs [3]ratio, factor float64) [3]ratio {
	for i := range rs {
		rs[i] = rs[i].scaled(factor)
	}
	return rs
}

// Estimate predicts the output size of info converted to t before any encode. It is a formula,
// never a measurement. Quality mode gives a range, and copy, target size, and bitrate modes
// give a point. size is the target size the user asked for when t aims for one and holds none,
// and hardware is true when the plan runs on the Apple media engine, whose ratios were measured
// on their own. GPU encoders on Windows share the software encoders' quality scale and take
// the software ratios. It returns false when the formula has nothing to go on.
func Estimate(info probe.Info, t format.Target, size int64, hardware bool) (SizeRange, bool) {
	_, hasAudio := info.FirstAudio()
	if t.Video.Codec == format.Copy && (t.Audio.Codec == format.Copy || !hasAudio) {
		return SizeRange{Low: info.Size, High: info.Size}, info.Size > 0
	}
	if t.Video.RateControl == format.RateTwoPass && t.Video.Bitrate == 0 {
		if t.TargetBytes != 0 {
			size = t.TargetBytes
		}
		return SizeRange{Low: size, High: size}, size != 0
	}
	if info.Duration <= 0 {
		return SizeRange{}, false
	}
	seconds := float64(info.Duration) / 1e9
	video, ok := estimateVideo(info, t.Video, hardware)
	if !ok {
		return SizeRange{}, false
	}
	audio := estimateAudio(info, t.Audio)
	bytes := func(videoBits float64) float64 { return (videoBits + audio) * seconds / 8 * estimateOverhead }
	low, high := bytes(video.Low), bytes(video.High)
	if low < 1 {
		return SizeRange{}, false
	}
	if video.Low == video.High {
		n := int64(math.Round(low))
		return SizeRange{Low: n, High: n}, true
	}
	return SizeRange{Low: int64(math.Round(low * (1 - spread))), High: int64(math.Round(high * (1 + spread)))}, true
}

// estimateVideo returns the expected video bitrate in bits per second as a low and high bound,
// equal outside quality mode and zero when the output carries no video. It returns false when
// the codec has no measured ratio or the source bitrate is unknown.
func estimateVideo(info probe.Info, v format.Video, hardware bool) (ratio, bool) {
	src, ok := info.FirstVideo()
	if v.Codec == "" || !ok {
		return ratio{}, true
	}
	srcVideo, known := sourceVideoBitrate(info)
	if v.Codec == format.Copy {
		return ratio{Low: srcVideo, High: srcVideo}, known
	}
	if v.RateControl != "" && v.RateControl != format.RateQuality {
		b := float64(v.Bitrate)
		return ratio{Low: b, High: b}, v.Bitrate != 0
	}
	table, measured := softwareRatios[v.Codec]
	if hw, ok := hardwareRatios[v.Codec]; hardware && ok {
		table, measured = hw, true
	}
	if !measured || !known {
		return ratio{}, false
	}
	level := min(max(int(v.Quality), int(format.QualityHigh)), len(table))
	return table[level-1].scaled(srcVideo * areaScale(src, v)), true
}

// sourceVideoBitrate returns the overall source bitrate less an assumed audio share, and false
// when the size or duration is unknown.
func sourceVideoBitrate(info probe.Info) (float64, bool) {
	if info.Size <= 0 || info.Duration <= 0 {
		return 0, false
	}
	total := float64(info.Size) * 8 / (float64(info.Duration) / 1e9)
	if _, ok := info.FirstAudio(); ok {
		return total - min(assumedAudioBitrate, total/2), true
	}
	return total, true
}

// areaScale returns the bitrate factor of the output frame against the source frame.
func areaScale(src probe.Stream, v format.Video) float64 {
	sw, sh := src.DisplaySize()
	ow, oh, ok := estimateFrame(src, v)
	if !ok {
		return 1
	}
	return math.Pow(float64(ow*oh)/float64(sw*sh), areaExponent)
}

// estimateFrame returns the output frame size for a source stream: the exact size when both
// sides are set, else the source fit into the box, as Build fits it. It returns false when the
// source size is unknown.
func estimateFrame(src probe.Stream, v format.Video) (int, int, bool) {
	w, h := src.DisplaySize()
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	switch {
	case v.Codec == format.Copy:
		return w, h, true
	case v.Width > 0 && v.Height > 0:
		return v.Width, v.Height, true
	}
	ow, oh, _ := fit(w, h, v.MaxWidth, v.MaxHeight)
	return ow, oh, true
}

// estimateAudio returns the expected audio bitrate in bits per second, zero when the output
// carries no audio.
func estimateAudio(info probe.Info, a format.Audio) float64 {
	src, ok := info.FirstAudio()
	if a.Codec == "" || !ok {
		return 0
	}
	if a.Codec == format.Copy {
		return assumedAudioBitrate
	}
	channels := src.Channels
	if a.Channels > 0 {
		channels = min(a.Channels, channels)
	}
	rate := float64(a.SampleRate)
	if rate == 0 {
		rate = estimateSampleRate
	}
	depth := a.BitDepth
	if depth == 0 {
		depth = 16
	}
	pcm := rate * float64(channels) * float64(depth)
	switch {
	case strings.HasPrefix(a.Codec, "pcm_"):
		return pcm
	case a.Codec == "flac" || a.Codec == "alac":
		return pcm * losslessShare
	case a.Bitrate != 0:
		return float64(a.Bitrate)
	}
	return defaultAudioBitrate
}
