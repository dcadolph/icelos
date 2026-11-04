package engine

import (
	"fmt"
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

// swiftEstimates holds what the Mac app's Estimate.bytes answers for each case in
// estimateCases, captured from the app's own code on 2026-11-03.
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
	// Size is the target size the user asked for.
	Size int64
	// Hardware is true for the media engine.
	Hardware bool
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
			cases = append(cases, estimateCase{Label: fmt.Sprintf("cal/%s/%v", id, hw), Info: calibrationClip(),
				Target: builtinTarget(t, id), Size: 25_000_000, Hardware: hw})
		}
	}
	fixtures := []struct {
		Name string
		Size int64
	}{{Name: "clean", Size: 11_653_883}, {Name: "grainy", Size: 43_393_755}}
	for _, fx := range fixtures {
		for _, id := range []string{"apple", "compatible", "web", "tiny"} {
			for _, hw := range []bool{false, true} {
				cases = append(cases, estimateCase{Label: fmt.Sprintf("%s/%s/%v", fx.Name, id, hw),
					Info: estimateClip(20*time.Second, fx.Size), Target: builtinTarget(t, id), Hardware: hw})
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
			Hardware: true},
		estimateCase{Label: "av1/hw", Info: cal, Target: with("web", func(tg *format.Target) { tg.Video.Codec = "av1" }),
			Hardware: true},
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
		}), Size: 25_000_000},
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
			r, gotOK := Estimate(test.Info, test.Target, test.Size, test.Hardware)
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

func TestSizeRangeSaving(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         SizeRange
		Source     int64
		WantResult int
		WantOK     bool
		WantPoint  bool
	}{{ // Test 0: Smaller, from the high bound.
		In: SizeRange{Low: 8_000_000, High: 48_000_000}, Source: 165_000_000, WantResult: 71, WantOK: true,
	}, { // Test 1: Larger is negative.
		In: SizeRange{Low: 30, High: 30}, Source: 20, WantResult: -50, WantOK: true, WantPoint: true,
	}, { // Test 2: Within half a percent is the same size.
		In: SizeRange{Low: 1004, High: 1004}, Source: 1000, WantOK: true, WantPoint: true,
	}, { // Test 3: Unknown source size.
		In: SizeRange{Low: 1, High: 2},
	}, { // Test 4: Empty estimate.
		Source: 100, WantPoint: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, ok := test.In.Saving(test.Source)
			want := [3]any{test.WantResult, test.WantOK, test.WantPoint}
			if diff := cmp.Diff(want, [3]any{got, ok, test.In.Point()}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
