package engine

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// testAvail is an encoder set with everything a full macOS ffmpeg build offers.
func testAvail() EncoderSet {
	return EncoderSet{
		"libx264": true, "libx265": true, "h264_videotoolbox": true, "hevc_videotoolbox": true,
		"aac": true, "aac_at": true, "libmp3lame": true,
	}
}

// testInfo returns a 10 second 4K landscape clip with stereo AAC.
func testInfo() probe.Info {
	return probe.Info{Duration: 10 * time.Second, Streams: []probe.Stream{
		{Index: 0, Kind: "video", Codec: "h264", Width: 3840, Height: 2160},
		{Index: 1, Kind: "audio", Codec: "aac", Channels: 6},
	}}
}

// testPreset looks up a built-in preset or fails the test.
func testPreset(t *testing.T, id string) preset.Preset {
	t.Helper()
	p, err := preset.Lookup(id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// head is the argument prefix every pass shares for input "in.mov".
var head = []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-nostats", //nolint:gochecknoglobals // Not modified, simplifies testing.
	"-progress", "pipe:1", "-y", "-i", "in.mov"}

func TestBuild(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	tests := []struct {
		Want         error
		Preset       string
		TargetBytes  int64
		Hardware     bool
		Info         func(probe.Info) probe.Info
		Avail        func(EncoderSet) EncoderSet
		WantPasses   [][]string
		WantEncoder  string
		WantHardware bool
	}{{ // Test 0: Apple preset on hardware keeps 4K and tags HEVC for QuickTime.
		Preset: "apple", Hardware: true, WantEncoder: "hevc_videotoolbox", WantHardware: true,
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "hevc_videotoolbox",
			"-pix_fmt", "yuv420p", "-tag:v", "hvc1", "-q:v", "65", "-map", "0:1", "-c:a", "aac_at",
			"-b:a", "192000", "-map_metadata", "0", "-movflags", "+faststart", "out.mp4"})},
	}, { // Test 1: Web preset in software scales to 1080p and downmixes.
		Preset: "web", WantEncoder: "libx264",
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264",
			"-preset", "medium", "-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-crf", "23",
			"-map", "0:1", "-c:a", "aac_at", "-b:a", "128000", "-ac", "2",
			"-map_metadata", "0", "-movflags", "+faststart", "out.mp4"})},
	}, { // Test 2: Hardware requested but missing falls to software and native AAC.
		Preset: "tiny", Hardware: true, WantEncoder: "libx265",
		Avail: func(s EncoderSet) EncoderSet { delete(s, "hevc_videotoolbox"); delete(s, "aac_at"); return s },
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx265",
			"-preset", "medium", "-vf", "scale=1280:720", "-pix_fmt", "yuv420p", "-tag:v", "hvc1",
			"-crf", "29", "-x265-params", "log-level=error", "-map", "0:1", "-c:a", "aac",
			"-b:a", "96000", "-ac", "2", "-map_metadata", "0", "-movflags", "+faststart", "out.mp4"})},
	}, { // Test 3: Target size runs two software passes even when hardware is preferred.
		Preset: preset.TargetSizeID, TargetBytes: 10_000_000, Hardware: true, WantEncoder: "libx264",
		WantPasses: [][]string{
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264", "-preset", "medium",
				"-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-b:v", "7632000",
				"-pass", "1", "-passlogfile", "plog", "-an", "-f", "null", os.DevNull}),
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264", "-preset", "medium",
				"-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-b:v", "7632000",
				"-pass", "2", "-passlogfile", "plog", "-map", "0:1", "-c:a", "aac_at",
				"-b:a", "128000", "-ac", "2", "-map_metadata", "0", "-movflags", "+faststart", "out.mp4"}),
		},
	}, { // Test 4: Remux copies both streams and tags HEVC.
		Preset: "remux",
		Info:   func(i probe.Info) probe.Info { i.Streams[0].Codec = "hevc"; return i },
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "copy",
			"-tag:v", "hvc1", "-map", "0:1", "-c:a", "copy",
			"-map_metadata", "0", "-movflags", "+faststart", "out.mp4"})},
	}, { // Test 5: Audio preset maps only the audio stream.
		Preset: "audio",
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:1", "-c:a", "aac_at",
			"-b:a", "192000", "-map_metadata", "0", "-movflags", "+faststart", "out.mp4"})},
	}, { // Test 6: MP3 skips cover art and faststart.
		Preset: "mp3",
		Info: func(probe.Info) probe.Info {
			return probe.Info{Duration: time.Minute, Streams: []probe.Stream{
				{Index: 0, Kind: "video", Codec: "mjpeg", Disposition: probe.Disposition{AttachedPic: 1}},
				{Index: 1, Kind: "audio", Codec: "flac", Channels: 2},
			}}
		},
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:1", "-c:a", "libmp3lame",
			"-b:a", "192000", "-map_metadata", "0", "out.mp4"})},
	}, { // Test 7: Video preset on an audio file.
		Preset: "apple", Want: ErrNoVideo,
		Info: func(i probe.Info) probe.Info { i.Streams = i.Streams[1:]; return i },
	}, { // Test 8: Audio preset on a silent file.
		Preset: "audio", Want: ErrNoAudio,
		Info: func(i probe.Info) probe.Info { i.Streams = i.Streams[:1]; return i },
	}, { // Test 9: Silent video drops the audio map.
		Preset: "compatible", WantEncoder: "libx264",
		Info: func(i probe.Info) probe.Info {
			i.Streams = []probe.Stream{{Index: 0, Kind: "video", Width: 1280, Height: 720}}
			return i
		},
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264",
			"-preset", "medium", "-pix_fmt", "yuv420p", "-crf", "20",
			"-map_metadata", "0", "-movflags", "+faststart", "out.mp4"})},
	}, { // Test 10: Software encoder missing.
		Preset: "web", Want: ErrMissingEncoder,
		Avail: func(s EncoderSet) EncoderSet { delete(s, "libx264"); return s },
	}, { // Test 11: MP3 encoder missing.
		Preset: "mp3", Want: ErrMissingEncoder,
		Avail: func(s EncoderSet) EncoderSet { delete(s, "libmp3lame"); return s },
	}, { // Test 12: Target size without a duration.
		Preset: preset.TargetSizeID, TargetBytes: 10_000_000, Want: ErrNoDuration,
		Info: func(i probe.Info) probe.Info { i.Duration = 0; return i },
	}, { // Test 13: Target size too small for the length.
		Preset: preset.TargetSizeID, TargetBytes: 100_000, Want: ErrTargetTooSmall,
	}, { // Test 14: Target size preset with no size is invalid.
		Preset: "remux", TargetBytes: 1, Want: preset.ErrInvalid,
	}, { // Test 15: Target size on HEVC uses x265 stats.
		Preset: "tiny", TargetBytes: 5_000_000, WantEncoder: "libx265",
		WantPasses: [][]string{
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx265", "-preset", "medium",
				"-vf", "scale=1280:720", "-pix_fmt", "yuv420p", "-tag:v", "hvc1", "-b:v", "3784000",
				"-x265-params", "pass=1:stats=plog:log-level=error", "-an", "-f", "null", os.DevNull}),
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx265", "-preset", "medium",
				"-vf", "scale=1280:720", "-pix_fmt", "yuv420p", "-tag:v", "hvc1", "-b:v", "3784000",
				"-x265-params", "pass=2:stats=plog:log-level=error", "-map", "0:1", "-c:a", "aac_at",
				"-b:a", "96000", "-ac", "2", "-map_metadata", "0", "-movflags", "+faststart", "out.mp4"}),
		},
	}, { // Test 16: Portrait phone video scales inside the rotated box.
		Preset: "web", WantEncoder: "libx264",
		Info: func(i probe.Info) probe.Info {
			i.Streams[0].SideData = []probe.SideData{{Rotation: -90}}
			return i
		},
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264",
			"-preset", "medium", "-vf", "scale=1080:1920", "-pix_fmt", "yuv420p", "-crf", "23",
			"-map", "0:1", "-c:a", "aac_at", "-b:a", "128000", "-ac", "2",
			"-map_metadata", "0", "-movflags", "+faststart", "out.mp4"})},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			p := testPreset(t, test.Preset)
			if test.TargetBytes != 0 {
				p = p.WithTargetSize(test.TargetBytes)
			}
			info, avail := testInfo(), testAvail()
			if test.Info != nil {
				info = test.Info(info)
			}
			if test.Avail != nil {
				avail = test.Avail(avail)
			}
			job := Job{Input: "in.mov", Output: "out.mp4", Preset: p, Hardware: test.Hardware}
			got, err := Build(job, info, avail, "out.mp4", "plog")
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			want := Plan{Passes: test.WantPasses, Encoder: test.WantEncoder, Hardware: test.WantHardware}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTargetVideoBitrate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want        error
		Target      int64
		Dur         time.Duration
		Audio       int
		WantBitrate int
	}{{ // Test 0: 25 MB over a minute with 128k audio.
		Target: 25_000_000, Dur: time.Minute, Audio: 128_000, WantBitrate: 3_105_333,
	}, { // Test 1: No audio.
		Target: 1_000_000, Dur: 10 * time.Second, WantBitrate: 776_000,
	}, { // Test 2: Zero duration.
		Target: 1_000_000, Want: ErrNoDuration,
	}, { // Test 3: Audio eats the whole budget.
		Target: 1_000_000, Dur: time.Minute, Audio: 192_000, Want: ErrTargetTooSmall,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := TargetVideoBitrate(test.Target, test.Dur, test.Audio)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantBitrate, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         [4]int
		WantSize   [2]int
		WantScaled bool
	}{{ // Test 0: 4K into 1080p.
		In: [4]int{3840, 2160, 1920, 1080}, WantSize: [2]int{1920, 1080}, WantScaled: true,
	}, { // Test 1: Already small enough.
		In: [4]int{1280, 720, 1920, 1080}, WantSize: [2]int{1280, 720},
	}, { // Test 2: Portrait uses the rotated box.
		In: [4]int{2160, 3840, 1920, 1080}, WantSize: [2]int{1080, 1920}, WantScaled: true,
	}, { // Test 3: Odd source is evened without upscaling.
		In: [4]int{641, 481, 0, 0}, WantSize: [2]int{640, 480}, WantScaled: true,
	}, { // Test 4: Ultra-wide is bounded by the long side.
		In: [4]int{5120, 1440, 1920, 1080}, WantSize: [2]int{1920, 540}, WantScaled: true,
	}, { // Test 5: Square is bounded by the short side.
		In: [4]int{2000, 2000, 1920, 1080}, WantSize: [2]int{1080, 1080}, WantScaled: true,
	}, { // Test 6: Unknown size is left alone.
		In: [4]int{0, 0, 1920, 1080}, WantSize: [2]int{0, 0},
	}, { // Test 7: Only one bound set.
		In: [4]int{3000, 1000, 1500, 0}, WantSize: [2]int{1500, 500}, WantScaled: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			w, h, scaled := fit(test.In[0], test.In[1], test.In[2], test.In[3])
			if diff := cmp.Diff(test.WantSize, [2]int{w, h}); diff != "" {
				t.Errorf("size mismatch (-want +got):\n%s", diff)
			}
			if scaled != test.WantScaled {
				t.Errorf("scaled = %v, want %v", scaled, test.WantScaled)
			}
		})
	}
}

func TestQualityIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In        preset.Quality
		WantIndex int
	}{{ // Test 0: High.
		In: preset.QualityHigh, WantIndex: 0,
	}, { // Test 1: Small.
		In: preset.QualitySmall, WantIndex: 2,
	}, { // Test 2: Unset is balanced.
		In: 0, WantIndex: 1,
	}, { // Test 3: Out of range is balanced.
		In: 9, WantIndex: 1,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := qualityIndex(test.In); got != test.WantIndex {
				t.Errorf("index = %d, want %d", got, test.WantIndex)
			}
		})
	}
}

func TestAudioCodecArgsInvalid(t *testing.T) {
	t.Parallel()
	_, err := audioCodecArgs(preset.Audio{Codec: "vorbis"}, probe.Stream{}, testAvail())
	if !errors.Is(err, preset.ErrInvalid) {
		t.Errorf("error = %v, want %v", err, preset.ErrInvalid)
	}
}
