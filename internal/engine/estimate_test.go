package engine

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// estimateWant is one expected estimate.
type estimateWant struct {
	// Low is the expected low bound in bytes.
	Low int64
	// High is the expected high bound in bytes.
	High int64
	// OK is true when an estimate is expected.
	OK bool
}

// swiftEstimates holds what the Mac app's Estimate.bytes answered for each case in
// estimateCases, captured from the app's own code on 2026-11-03 before the formula moved here.
// A label ending in true or hw ran with the media engine on.
var swiftEstimates = map[string]estimateWant{ //nolint:gochecknoglobals // Not modified, simplifies testing.
	"cal/apple/false":         {Low: 8506978, High: 23993434, OK: true},
	"cal/apple/true":          {Low: 9390355, High: 24533276, OK: true},
	"cal/compatible/false":    {Low: 20088976, High: 104649727, OK: true},
	"cal/compatible/true":     {Low: 30116947, High: 95712345, OK: true},
	"cal/web/false":           {Low: 15639307, High: 33330544, OK: true},
	"cal/web/true":            {Low: 9766482, High: 23833326, OK: true},
	"cal/tiny/false":          {Low: 2059180, High: 6935449, OK: true},
	"cal/tiny/true":           {Low: 2183845, High: 6706897, OK: true},
	"cal/target-size/false":   {Low: 25000000, High: 25000000, OK: true},
	"cal/target-size/true":    {Low: 25000000, High: 25000000, OK: true},
	"cal/remux/false":         {Low: 181765278, High: 181765278, OK: true},
	"cal/remux/true":          {Low: 181765278, High: 181765278, OK: true},
	"cal/audio/false":         {Low: 2181600, High: 2181600, OK: true},
	"cal/audio/true":          {Low: 2181600, High: 2181600, OK: true},
	"cal/mp3/false":           {Low: 2181600, High: 2181600, OK: true},
	"cal/mp3/true":            {Low: 2181600, High: 2181600, OK: true},
	"clean/apple/false":       {Low: 845511, High: 1883611, OK: true},
	"clean/apple/true":        {Low: 900752, High: 1917369, OK: true},
	"clean/compatible/false":  {Low: 1517519, High: 6863473, OK: true},
	"clean/compatible/true":   {Low: 2144605, High: 6304586, OK: true},
	"clean/web/false":         {Low: 1187009, High: 2339756, OK: true},
	"clean/web/true":          {Low: 819760, High: 1745861, OK: true},
	"clean/tiny/false":        {Low: 285537, High: 625306, OK: true},
	"clean/tiny/true":         {Low: 293333, High: 611014, OK: true},
	"grainy/apple/false":      {Low: 1999573, High: 5692015, OK: true},
	"grainy/apple/true":       {Low: 2210612, High: 5820983, OK: true},
	"grainy/compatible/false": {Low: 4771973, High: 24967496, OK: true},
	"grainy/compatible/true":  {Low: 7167658, High: 22832353, OK: true},
	"grainy/web/false":        {Low: 3714404, High: 7935994, OK: true},
	"grainy/web/true":         {Low: 2311384, High: 5667106, OK: true},
	"grainy/tiny/false":       {Low: 475565, High: 1636869, OK: true},
	"grainy/tiny/true":        {Low: 505348, High: 1582268, OK: true},
	"cbr":                     {Low: 47268000, High: 47268000, OK: true},
	"pcm":                     {Low: 16034760, High: 16034760, OK: true},
	"flac":                    {Low: 15707520, High: 15707520, OK: true},
	"uhd/compatible":          {Low: 8160241, High: 38292030, OK: true},
	"uhd/web/hw":              {Low: 4299146, High: 9460564, OK: true},
	"av1/hw":                  {Low: 11340203, High: 23811333, OK: true},
	"vp9":                     {Low: 12773238, High: 26984403, OK: true},
	"mpeg2":                   {},
	"short":                   {},
	"wide/tiny":               {Low: 1327390, High: 3039959, OK: true},
	"rot/compatible":          {Low: 8160241, High: 38292030, OK: true},
	"exact/web":               {Low: 4066837, High: 7706417, OK: true},
	"sized":                   {Low: 8000000, High: 8000000, OK: true},
	"target/none":             {},
	"q9":                      {Low: 9864635, High: 14596033, OK: true},
	"mono":                    {Low: 2181600, High: 2181600, OK: true},
	"silent/web":              {Low: 14473678, High: 32048072, OK: true},
	"silent/remux":            {Low: 181765278, High: 181765278, OK: true},
}

// estimateCase is one input to Estimate.
type estimateCase struct {
	// Label names the case in swiftEstimates.
	Label string
	// Info is the source probe.
	Info probe.Info
	// Target is the output settings.
	Target format.Target
	// Encoder is the video encoder the plan picked.
	Encoder string
}

// testEngineEncoder returns the video encoder a plan picks for t with the media engine on or off:
// VideoToolbox for H.264 and HEVC when on, else a software encoder, which shares the codec's
// software ratios.
func testEngineEncoder(t format.Target, on bool) string {
	if on && (t.Video.Codec == "h264" || t.Video.Codec == "hevc") {
		return t.Video.Codec + "_videotoolbox"
	}
	return ""
}

// estimateClip returns the probe of a 1920 by 1080 H.264 clip with stereo AAC.
func estimateClip(dur time.Duration, size int64) probe.Info {
	return probe.Info{Format: "mov,mp4,m4a,3gp,3g2,mj2", Duration: dur, Size: size, Streams: []probe.Stream{
		{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Kind: "audio", Codec: "aac", Channels: 2},
	}}
}

// calibrationClip returns the 90 second calibration clip the Mac app's tests use.
func calibrationClip() probe.Info {
	return estimateClip(90*time.Second, 181_765_278)
}

// builtinTarget returns the target of a built-in preset.
func builtinTarget(t *testing.T, id string) format.Target {
	t.Helper()
	p, err := preset.Lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	return p.Target
}

// estimateCases returns the inputs whose answers swiftEstimates holds.
func estimateCases(t *testing.T) []estimateCase { //nolint:funlen // Test table.
	t.Helper()
	ids := []string{"apple", "compatible", "web", "tiny", "target-size", "remux", "audio", "mp3"}
	var cases []estimateCase
	for _, id := range ids {
		for _, hw := range []bool{false, true} {
			tg := builtinTarget(t, id)
			if id == preset.TargetSizeID {
				tg.TargetBytes = 25_000_000
			}
			cases = append(cases, estimateCase{Label: fmt.Sprintf("cal/%s/%v", id, hw), Info: calibrationClip(),
				Target: tg, Encoder: testEngineEncoder(tg, hw)})
		}
	}
	fixtures := []struct {
		Name string
		Size int64
	}{{Name: "clean", Size: 11_653_883}, {Name: "grainy", Size: 43_393_755}}
	for _, fx := range fixtures {
		for _, id := range []string{"apple", "compatible", "web", "tiny"} {
			for _, hw := range []bool{false, true} {
				tg := builtinTarget(t, id)
				cases = append(cases, estimateCase{Label: fmt.Sprintf("%s/%s/%v", fx.Name, id, hw),
					Info: estimateClip(20*time.Second, fx.Size), Target: tg, Encoder: testEngineEncoder(tg, hw)})
			}
		}
	}
	cal := calibrationClip()
	with := func(id string, edit func(*format.Target)) format.Target {
		tg := builtinTarget(t, id)
		edit(&tg)
		return tg
	}
	resized := func(w, h int, rotation int) probe.Info {
		info := calibrationClip()
		info.Streams[0].Width, info.Streams[0].Height = w, h
		if rotation != 0 {
			info.Streams[0].SideData = []probe.SideData{{Rotation: rotation}}
		}
		return info
	}
	short := calibrationClip()
	short.Duration = 0
	silent := calibrationClip()
	silent.Streams = silent.Streams[:1]
	return append(cases,
		estimateCase{Label: "cbr", Info: cal, Target: with("compatible", func(tg *format.Target) {
			tg.Video.RateControl, tg.Video.Bitrate = format.RateCBR, 4_000_000
		})},
		estimateCase{Label: "pcm", Info: cal, Target: with("audio", func(tg *format.Target) {
			tg.Audio = format.Audio{Codec: "pcm_s16le", SampleRate: 44_100}
		})},
		estimateCase{Label: "flac", Info: cal, Target: with("audio", func(tg *format.Target) {
			tg.Audio = format.Audio{Codec: "flac", BitDepth: 24}
		})},
		estimateCase{Label: "uhd/compatible", Info: resized(3840, 2160, 0), Target: builtinTarget(t, "compatible")},
		estimateCase{Label: "uhd/web/hw", Info: resized(3840, 2160, 0), Target: builtinTarget(t, "web"),
			Encoder: testEngineEncoder(builtinTarget(t, "web"), true)},
		estimateCase{Label: "av1/hw", Info: cal, Target: with("web", func(tg *format.Target) { tg.Video.Codec = "av1" }),
			Encoder: "libsvtav1"},
		estimateCase{Label: "vp9", Info: cal, Target: with("web", func(tg *format.Target) { tg.Video.Codec = "vp9" })},
		estimateCase{Label: "mpeg2", Info: cal, Target: with("compatible", func(tg *format.Target) {
			tg.Video.Codec = "mpeg2video"
		})},
		estimateCase{Label: "short", Info: short, Target: builtinTarget(t, "web")},
		estimateCase{Label: "wide/tiny", Info: resized(4096, 1716, 0), Target: builtinTarget(t, "tiny")},
		estimateCase{Label: "rot/compatible", Info: resized(3840, 2160, -90), Target: builtinTarget(t, "compatible")},
		estimateCase{Label: "exact/web", Info: cal, Target: with("web", func(tg *format.Target) {
			tg.Video.Width, tg.Video.Height = 640, 360
		})},
		estimateCase{Label: "sized", Info: cal, Target: with("target-size", func(tg *format.Target) {
			tg.TargetBytes = 8_000_000
		})},
		estimateCase{Label: "target/none", Info: cal, Target: builtinTarget(t, "target-size")},
		estimateCase{Label: "q9", Info: cal, Target: with("web", func(tg *format.Target) { tg.Video.Quality = 9 })},
		estimateCase{Label: "mono", Info: cal, Target: with("mp3", func(tg *format.Target) { tg.Audio.Channels = 1 })},
		estimateCase{Label: "silent/web", Info: silent, Target: builtinTarget(t, "web")},
		estimateCase{Label: "silent/remux", Info: silent, Target: builtinTarget(t, "remux")},
	)
}

// TestEstimate checks that every case answers the bytes the Mac app's formula answers.
func TestEstimate(t *testing.T) {
	t.Parallel()
	cases := estimateCases(t)
	if len(cases) != len(swiftEstimates) {
		t.Fatalf("%d cases, %d captured answers", len(cases), len(swiftEstimates))
	}
	for _, test := range cases {
		t.Run(test.Label, func(t *testing.T) {
			t.Parallel()
			want, ok := swiftEstimates[test.Label]
			if !ok {
				t.Fatalf("no captured answer for %s", test.Label)
			}
			r, gotOK := Estimate(test.Info, test.Target, test.Encoder)
			got := estimateWant{Low: r.Low, High: r.High, OK: gotOK}
			if !gotOK {
				got = estimateWant{}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// measuredPoints holds the output sizes of the calibration clip through `icelos convert` with
// the media engine on and a 25MB target, from the 2026-11-02 run, for the presets the estimate
// answers with one size.
var measuredPoints = map[string]int64{ //nolint:gochecknoglobals // Not modified, simplifies testing.
	"target-size": 24_364_457,
	"remux":       181_765_227,
	"audio":       2_173_256,
	"mp3":         2_155_527,
}

// qualityFixture is a 20 second quality calibration clip with its measured output sizes.
type qualityFixture struct {
	// Name tells the fixtures apart in test output.
	Name string
	// Info is the clip's probe.
	Info probe.Info
	// Software holds output sizes by preset ID with the media engine off.
	Software map[string]int64
	// Hardware holds output sizes by preset ID with the media engine on.
	Hardware map[string]int64
}

// qualityFixtures returns the two 20 second clips from the wolf-eye-bird reference looped,
// encoded with h264_videotoolbox at 16 Mb/s and AAC at 256 kb/s, clean and with
// `noise=alls=6:allf=t`, and the 2026-11-03 sizes of the quality presets through `icelos serve`.
func qualityFixtures() []qualityFixture {
	return []qualityFixture{{
		Name: "clean", Info: estimateClip(20*time.Second, 11_653_883),
		Software: map[string]int64{"apple": 1_724_081, "compatible": 1_698_516, "web": 1_331_200, "tiny": 605_143},
		Hardware: map[string]int64{"apple": 1_754_605, "compatible": 2_394_222, "web": 1_599_243, "tiny": 603_763},
	}, {
		Name: "grainy", Info: estimateClip(20*time.Second, 43_393_755),
		Software: map[string]int64{"apple": 2_234_214, "compatible": 22_710_486, "web": 7_228_001, "tiny": 560_670},
		Hardware: map[string]int64{"apple": 2_469_503, "compatible": 20_770_089, "web": 2_582_508, "tiny": 605_796},
	}}
}

// qualityPresetIDs are the built-in presets that use the quality rate control.
func qualityPresetIDs() []string {
	return []string{"apple", "compatible", "web", "tiny"}
}

// keptRatio inverts the estimate formula: the share of the source video bitrate an output of
// bytes kept, for the same audio, frame, and overhead terms the estimate uses.
func keptRatio(t *testing.T, bytes int64, info probe.Info, tg format.Target) float64 {
	t.Helper()
	src, ok := sourceVideoBitrate(info)
	video, vok := info.FirstVideo()
	if !ok || !vok {
		t.Fatal("calibration clip has no source bitrate or video")
	}
	seconds := float64(info.Duration) / 1e9
	bits := float64(bytes)*8/seconds/estimateOverhead - estimateAudio(info, tg.Audio)
	return bits / src / areaScale(video, tg.Video)
}

func TestEstimatePointNearMeasured(t *testing.T) {
	t.Parallel()
	for id, want := range measuredPoints {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			tg := builtinTarget(t, id)
			if id == preset.TargetSizeID {
				tg.TargetBytes = 25_000_000
			}
			got, ok := Estimate(calibrationClip(), tg, testEngineEncoder(tg, true))
			if !ok || got.Low != got.High {
				t.Fatalf("estimate = %+v %v, want one size", got, ok)
			}
			if off := math.Abs(float64(got.High)/float64(want) - 1); off > 0.35 {
				t.Errorf("estimate %d is %.0f%% off the measured %d, want within 35%%", got.High, off*100, want)
			}
		})
	}
}

func TestEstimateRangeHoldsMeasured(t *testing.T) {
	t.Parallel()
	for _, id := range qualityPresetIDs() {
		for _, hw := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", id, hw), func(t *testing.T) {
				t.Parallel()
				tg := builtinTarget(t, id)
				for _, fx := range qualityFixtures() {
					want := fx.Software[id]
					if hw {
						want = fx.Hardware[id]
					}
					got, ok := Estimate(fx.Info, tg, testEngineEncoder(tg, hw))
					if !ok || got.Low == got.High {
						t.Fatalf("%s: estimate = %+v %v, want a range", fx.Name, got, ok)
					}
					if want < got.Low || want > got.High {
						t.Errorf("%s: measured %d outside %d to %d", fx.Name, want, got.Low, got.High)
					}
				}
			})
		}
	}
}

func TestEstimateBoundsNearMeasured(t *testing.T) {
	t.Parallel()
	cal := calibrationClip()
	src, _ := sourceVideoBitrate(cal)
	video, _ := cal.FirstVideo()
	for _, id := range qualityPresetIDs() {
		for _, hw := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", id, hw), func(t *testing.T) {
				t.Parallel()
				tg := builtinTarget(t, id)
				var kept []float64
				for _, fx := range qualityFixtures() {
					size := fx.Software[id]
					if hw {
						size = fx.Hardware[id]
					}
					kept = append(kept, keptRatio(t, size, fx.Info, tg))
				}
				bounds, ok := estimateVideo(cal, tg.Video, testEngineEncoder(tg, hw))
				if !ok {
					t.Fatal("no video bounds")
				}
				area := areaScale(video, tg.Video)
				for _, bound := range []float64{bounds.Low / src / area, bounds.High / src / area} {
					nearest := math.Inf(1)
					for _, k := range kept {
						nearest = min(nearest, math.Abs(math.Log(bound/k)))
					}
					if nearest > math.Log(2) {
						t.Errorf("bound %.4f is more than 2x from every measured ratio %v", bound, kept)
					}
				}
			})
		}
	}
}

func TestEstimateEncoderRatios(t *testing.T) {
	t.Parallel()
	web := builtinTarget(t, "web")
	av1 := web
	av1.Video.Codec = "av1"
	tests := []struct {
		Target    format.Target
		Encoder   string
		Other     string
		WantEqual bool
	}{{ // Test 0: The media engine has its own ratios.
		Target: web, Encoder: "h264_videotoolbox", Other: "libx264",
	}, { // Test 1: A GPU encoder shares the software ratios.
		Target: web, Encoder: "h264_nvenc", Other: "libx264", WantEqual: true,
	}, { // Test 2: No encoder at all falls back to the codec's software ratios.
		Target: web, Encoder: "", Other: "libx264", WantEqual: true,
	}, { // Test 3: AV1 has no measured encoder.
		Target: av1, Encoder: "libsvtav1", Other: "", WantEqual: true,
	}}
	cal := calibrationClip()
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			a, aok := estimateVideo(cal, test.Target.Video, test.Encoder)
			b, bok := estimateVideo(cal, test.Target.Video, test.Other)
			if !aok || !bok {
				t.Fatalf("bounds missing: %v %v", aok, bok)
			}
			if diff := cmp.Diff(test.WantEqual, a == b); diff != "" {
				t.Errorf("equal mismatch (-want +got):\n%s\n%+v against %+v", diff, a, b)
			}
		})
	}
}
