package engine

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/icelos/internal/format"
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
			"-b:a", "192000", "-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
	}, { // Test 1: Web preset in software scales to 1080p and downmixes.
		Preset: "web", WantEncoder: "libx264",
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264",
			"-preset", "medium", "-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-crf", "23",
			"-map", "0:1", "-c:a", "aac_at", "-b:a", "128000", "-ac", "2",
			"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
	}, { // Test 2: Hardware requested but missing falls to software and native AAC.
		Preset: "tiny", Hardware: true, WantEncoder: "libx265",
		Avail: func(s EncoderSet) EncoderSet { delete(s, "hevc_videotoolbox"); delete(s, "aac_at"); return s },
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx265",
			"-preset", "medium", "-vf", "scale=1280:720", "-pix_fmt", "yuv420p", "-tag:v", "hvc1",
			"-crf", "29", "-x265-params", "log-level=error", "-map", "0:1", "-c:a", "aac",
			"-b:a", "96000", "-ac", "2", "-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
	}, { // Test 3: Target size runs two software passes even when hardware is preferred.
		Preset: preset.TargetSizeID, TargetBytes: 10_000_000, Hardware: true, WantEncoder: "libx264",
		WantPasses: [][]string{
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264", "-preset", "medium",
				"-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-b:v", "7632000",
				"-pass", "1", "-passlogfile", "plog", "-an", "-f", "null", os.DevNull}),
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264", "-preset", "medium",
				"-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-b:v", "7632000",
				"-pass", "2", "-passlogfile", "plog", "-map", "0:1", "-c:a", "aac_at",
				"-b:a", "128000", "-ac", "2", "-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"}),
		},
	}, { // Test 4: Remux copies both streams and tags HEVC.
		Preset: "remux",
		Info:   func(i probe.Info) probe.Info { i.Streams[0].Codec = "hevc"; return i },
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "copy",
			"-tag:v", "hvc1", "-map", "0:1", "-c:a", "copy",
			"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
	}, { // Test 5: Audio preset maps only the audio stream.
		Preset: "audio",
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:1", "-c:a", "aac_at",
			"-b:a", "192000", "-map_metadata", "0", "-movflags", "+faststart", "-f", "ipod", "out.mp4"})},
	}, { // Test 6: MP3 skips cover art and faststart.
		Preset: "mp3",
		Info: func(probe.Info) probe.Info {
			return probe.Info{Duration: time.Minute, Streams: []probe.Stream{
				{Index: 0, Kind: "video", Codec: "mjpeg", Disposition: probe.Disposition{AttachedPic: 1}},
				{Index: 1, Kind: "audio", Codec: "flac", Channels: 2},
			}}
		},
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:1", "-c:a", "libmp3lame",
			"-b:a", "192000", "-map_metadata", "0", "-f", "mp3", "out.mp4"})},
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
			"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
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
				"-b:a", "96000", "-ac", "2", "-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"}),
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
			"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
	}, { // Test 17: Remux refuses video MP4 cannot carry before ffmpeg runs.
		Preset: "remux", Want: ErrCopy,
		Info: func(i probe.Info) probe.Info { i.Streams[0].Codec = "theora"; return i },
	}, { // Test 18: Remux refuses audio MP4 cannot carry before ffmpeg runs.
		Preset: "remux", Want: ErrCopy,
		Info: func(i probe.Info) probe.Info { i.Streams[1].Codec = "vorbis"; return i },
	}, { // Test 19: Remux of AVI MPEG-4 with B-frames copies through a raw stream to rebuild times.
		Preset: "remux",
		Info: func(i probe.Info) probe.Info {
			i.Format = "avi"
			i.Streams[0].Codec, i.Streams[0].BFrames, i.Streams[0].BaseFrameRate = "mpeg4", 1, "24/1"
			return i
		},
		WantPasses: [][]string{
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "copy", "-bsf:v", "mpeg4_unpack_bframes",
				"-f", "m4v", "plog.m4v"}),
			slices.Concat(head, []string{"-framerate", "24/1", "-i", "plog.m4v", "-map", "1:0",
				"-c:v", "copy", "-map", "0:1", "-c:a", "copy",
				"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"}),
		},
	}, { // Test 20: Remux of AVI MPEG-4 without B-frames copies in one pass.
		Preset: "remux",
		Info: func(i probe.Info) probe.Info {
			i.Format = "avi"
			i.Streams[0].Codec, i.Streams[0].BaseFrameRate = "mpeg4", "24/1"
			return i
		},
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "copy",
			"-map", "0:1", "-c:a", "copy",
			"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
	}, { // Test 21: Remux of MPEG-4 with B-frames from a container with times copies in one pass.
		Preset: "remux",
		Info: func(i probe.Info) probe.Info {
			i.Format = "mov,mp4,m4a,3gp,3g2,mj2"
			i.Streams[0].Codec, i.Streams[0].BFrames = "mpeg4", 1
			return i
		},
		WantPasses: [][]string{slices.Concat(head, []string{"-map", "0:0", "-c:v", "copy",
			"-map", "0:1", "-c:a", "copy",
			"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"})},
	}, { // Test 22: Target size spreads over the streams' playing time, not a longer container.
		Preset: preset.TargetSizeID, TargetBytes: 10_000_000, WantEncoder: "libx264",
		Info: func(i probe.Info) probe.Info {
			i.Duration = 20 * time.Second
			i.Streams[0].Duration, i.Streams[1].Tags.Duration = "10.000000", "00:00:10.000000000"
			return i
		},
		WantPasses: [][]string{
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264", "-preset", "medium",
				"-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-b:v", "7632000",
				"-pass", "1", "-passlogfile", "plog", "-an", "-f", "null", os.DevNull}),
			slices.Concat(head, []string{"-map", "0:0", "-c:v", "libx264", "-preset", "medium",
				"-vf", "scale=1920:1080", "-pix_fmt", "yuv420p", "-b:v", "7632000",
				"-pass", "2", "-passlogfile", "plog", "-map", "0:1", "-c:a", "aac_at",
				"-b:a", "128000", "-ac", "2", "-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "out.mp4"}),
		},
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
			if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(Plan{}, "Copied", "Fit")); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildFit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Preset         string
		TargetBytes    int64
		WantFit        bool
		WantBitrate    int
		WantAudioBytes int64
	}{{ // Test 0: Target size carries a fit at the planned bitrate with stereo AAC over 10 s.
		Preset: preset.TargetSizeID, TargetBytes: 10_000_000, WantFit: true, WantBitrate: 7_632_000,
		WantAudioBytes: 160_000,
	}, { // Test 1: A quality preset has no fit.
		Preset: "web",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			p := testPreset(t, test.Preset)
			if test.TargetBytes != 0 {
				p = p.WithTargetSize(test.TargetBytes)
			}
			job := Job{Input: "in.mov", Output: "out.mp4", Preset: p}
			got, err := Build(job, testInfo(), testAvail(), "out.mp4", "plog")
			if err != nil {
				t.Fatal(err)
			}
			if (got.Fit != nil) != test.WantFit {
				t.Fatalf("fit = %v, want fit %v", got.Fit, test.WantFit)
			}
			if got.Fit == nil {
				return
			}
			if diff := cmp.Diff(test.WantBitrate, got.Fit.Bitrate); diff != "" {
				t.Errorf("bitrate mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantAudioBytes, got.Fit.AudioBytes); diff != "" {
				t.Errorf("audio bytes mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(got.Passes[1], got.Fit.Pass(got.Fit.Bitrate)); diff != "" {
				t.Errorf("rebuilt last pass mismatch (-want +got):\n%s", diff)
			}
			want := slices.Clone(got.Passes[1])
			want[slices.Index(want, "-b:v")+1] = "5000000"
			if diff := cmp.Diff(want, got.Fit.Pass(5_000_000)); diff != "" {
				t.Errorf("retry pass mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFitRetry(t *testing.T) {
	t.Parallel()
	fit := Fit{Target: 200_000, Bitrate: 616_960, AudioBytes: 33_333}
	tests := []struct {
		Want        error
		Fit         Fit
		Actual      int64
		WantBitrate int
	}{{ // Test 0: The video share is scaled by its miss, aimed inside the headroom.
		Fit: fit, Actual: 220_709, WantBitrate: 529_017,
	}, { // Test 1: Audio alone fills the limit.
		Fit: Fit{Target: 200_000, Bitrate: 500_000, AudioBytes: 200_000}, Actual: 230_000,
		Want: ErrTargetTooSmall,
	}, { // Test 2: The scaled bitrate falls under the floor.
		Fit: Fit{Target: 200_000, Bitrate: 70_000, AudioBytes: 33_333}, Actual: 2_000_000,
		Want: ErrTargetTooSmall,
	}, { // Test 3: An output no bigger than its audio share.
		Fit: fit, Actual: 33_333, Want: ErrTargetTooSmall,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := test.Fit.Retry(test.Actual)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantBitrate, got); diff != "" {
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
		In        format.Quality
		WantIndex int
	}{{ // Test 0: High.
		In: format.QualityHigh, WantIndex: 0,
	}, { // Test 1: Small.
		In: format.QualitySmall, WantIndex: 2,
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

func TestBuildRange(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	web := testPreset(t, "web")
	sized := testPreset(t, preset.TargetSizeID).WithTargetSize(5_000_000)
	half, err := TargetVideoBitrate(5_000_000, 5*time.Second, 128_000)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		Want        error
		Preset      preset.Preset
		Streams     Streams
		Range       Range
		WantRuns    [][]string
		WantMissing []string
	}{{ // Test 0: A middle span reads only that span of the input.
		Preset: web, Range: Range{Start: 2500 * time.Millisecond, End: 7 * time.Second},
		WantRuns: [][]string{{"-y", "-ss", "2.5", "-to", "7", "-i", "in.mov"}},
	}, { // Test 1: Start only.
		Preset: web, Range: Range{Start: 3 * time.Second},
		WantRuns: [][]string{{"-y", "-ss", "3", "-i", "in.mov"}}, WantMissing: []string{"-to"},
	}, { // Test 2: The whole input adds nothing.
		Preset: web, WantMissing: []string{"-ss", "-to"},
	}, { // Test 3: External audio is read over the same span.
		Preset: web, Range: Range{Start: time.Second, End: 4 * time.Second},
		Streams:  Streams{ExternalAudio: []string{"dub.wav"}},
		WantRuns: [][]string{{"-ss", "1", "-to", "4", "-i", "in.mov", "-ss", "1", "-to", "4", "-i", "dub.wav"}},
	}, { // Test 4: Burned text subtitles follow the seek.
		Preset: web, Range: Range{Start: 10 * time.Second},
		Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesBurn, External: "en.srt"}},
		WantRuns: [][]string{{"-vf", "scale=1920:1080,setpts=PTS+10/TB,subtitles=filename=en.srt," +
			"setpts=PTS-STARTPTS"}},
	}, { // Test 5: Burned subtitles from the start need no shift.
		Preset: web, Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesBurn, External: "en.srt"}},
		WantRuns: [][]string{{"-vf", "scale=1920:1080,subtitles=filename=en.srt"}}, WantMissing: []string{"setpts"},
	}, { // Test 6: A target size is budgeted over the span, on both passes.
		Preset: sized, Range: Range{Start: 5 * time.Second},
		WantRuns: [][]string{{"-ss", "5", "-i", "in.mov"}, {"-b:v", strconv.Itoa(half)}},
	}, { // Test 7: Backward span.
		Preset: web, Range: Range{Start: 4 * time.Second, End: 2 * time.Second}, Want: ErrRange,
	}, { // Test 8: Negative start.
		Preset: web, Range: Range{Start: -time.Second}, Want: ErrRange,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			job := Job{Input: "in.mov", Preset: test.Preset, Streams: test.Streams, Range: test.Range}
			extra := []probe.Info{{Streams: []probe.Stream{{Index: 0, Kind: "audio", Codec: "pcm_s16le",
				Channels: 2, SampleRate: 48000}}}}
			plan, err := Build(job, testInfo(), streamsAvail(), "out.mp4", "pass", extra...)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			for _, run := range test.WantRuns {
				if !containsRun(plan.Passes, run) {
					t.Errorf("passes %q lack %q", plan.Passes, run)
				}
			}
			for _, pass := range plan.Passes {
				for _, arg := range pass {
					for _, missing := range test.WantMissing {
						if strings.Contains(arg, missing) {
							t.Errorf("pass %q has %q", pass, missing)
						}
					}
				}
			}
			if test.Range.Start > 0 && len(plan.Passes) == 2 && !slices.Contains(plan.Passes[0], "-ss") {
				t.Errorf("first pass %q is not ranged", plan.Passes[0])
			}
		})
	}
}

func TestRangeSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In       Range
		Total    time.Duration
		WantSpan time.Duration
	}{
		{Total: 10 * time.Second, WantSpan: 10 * time.Second},                                   // Test 0: Whole.
		{In: Range{Start: 2 * time.Second}, Total: 10 * time.Second, WantSpan: 8 * time.Second}, // Test 1: Start only.
		{In: Range{Start: 2 * time.Second, End: 5 * time.Second}, WantSpan: 3 * time.Second},    // Test 2: Unknown total.
		{In: Range{End: 30 * time.Second}, Total: 10 * time.Second, WantSpan: 10 * time.Second}, // Test 3: End past total.
		{In: Range{Start: 20 * time.Second}, Total: 10 * time.Second, WantSpan: 0},              // Test 4: Start past total.
		{In: Range{Start: 2 * time.Second}, WantSpan: 0},                                        // Test 5: Nothing known.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := test.In.Span(test.Total); got != test.WantSpan {
				t.Errorf("span = %s, want %s", got, test.WantSpan)
			}
		})
	}
}
