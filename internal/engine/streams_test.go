package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// streamsInfo returns a clip with h264 video, two audio tracks, a text and a bitmap subtitle.
func streamsInfo() probe.Info {
	return probe.Info{Streams: []probe.Stream{
		{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Kind: "audio", Codec: "aac", Channels: 2, SampleRate: 48000},
		{Index: 2, Kind: "audio", Codec: "ac3", Channels: 6, SampleRate: 48000},
		{Index: 3, Kind: "subtitle", Codec: "subrip"},
		{Index: 4, Kind: "subtitle", Codec: "hdmv_pgs_subtitle"},
	}}
}

// streamsAvail is testAvail plus the subtitle encoders.
func streamsAvail() EncoderSet {
	set := testAvail()
	for _, name := range append([]string{"mpeg4", "libvpx-vp9", "libopus"}, subtitleEncoders...) {
		set[name] = true
	}
	return set
}

func TestStreamsValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want error
		In   Streams
	}{
		{In: Streams{}}, // Test 0: Zero value.
		{In: Streams{AudioTracks: []int{AllTracks}}},                                                  // Test 1: All tracks.
		{In: Streams{AudioTracks: []int{AllTracks, 1}}, Want: ErrStreams},                             // Test 2: All mixed in.
		{In: Streams{AudioTracks: []int{-2}}, Want: ErrStreams},                                       // Test 3: Negative track.
		{In: Streams{AudioTracks: []int{1, 1}}, Want: ErrStreams},                                     // Test 4: Duplicate.
		{In: Streams{ExternalAudio: make([]string, 7)}, Want: ErrStreams},                             // Test 5: Seven files.
		{In: Streams{ExternalAudio: []string{""}}, Want: ErrStreams},                                  // Test 6: Empty path.
		{In: Streams{Subtitles: Subtitles{Mode: "wild"}}, Want: ErrStreams},                           // Test 7: Unknown mode.
		{In: Streams{Subtitles: Subtitles{External: "a.srt"}}, Want: ErrStreams},                      // Test 8: File without mode.
		{In: Streams{Subtitles: Subtitles{Mode: SubtitlesPick, Track: -1}}, Want: ErrStreams},         // Test 9: Negative track.
		{In: Streams{Subtitles: Subtitles{Mode: SubtitlesBurn, External: "a.sup"}}, Want: ErrStreams}, // Test 10: Bitmap file.
		{In: Streams{Subtitles: Subtitles{Mode: SubtitlesKeep, External: "A.SRT"}}},                   // Test 11: Upper case ext.
		{In: Streams{Threads: -1}, Want: ErrStreams},                                                  // Test 12: Negative threads.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if err := test.In.Validate(); !errors.Is(err, test.Want) {
				t.Errorf("error = %v, want %v", err, test.Want)
			}
		})
	}
}

// streamsTarget returns the web preset moved to container box with video codec vc.
func streamsTarget(t *testing.T, box, vc string) preset.Preset {
	t.Helper()
	p := testPreset(t, "web")
	p.Container = box
	if vc != "" {
		p.Video.Codec = vc
	}
	return p
}

func TestBuildStreams(t *testing.T) { //nolint:funlen,maintidx // Test function.
	t.Parallel()
	web := func(t *testing.T) preset.Preset { t.Helper(); return testPreset(t, "web") }
	tests := []struct {
		Want        error
		Preset      func(*testing.T) preset.Preset
		Streams     Streams
		Extra       []probe.Info
		Info        func(probe.Info) probe.Info
		WantHas     [][]string
		WantMissing []string
		WantFilters []string
		WantPasses  int
	}{{ // Test 0: Default keeps the first audio track only and no subtitles.
		Preset:      web,
		WantHas:     [][]string{{"-map", "0:1", "-c:a", "aac_at"}, {"-map_metadata", "0"}},
		WantMissing: []string{"0:2", "-c:s:0", "-threads", "-map_chapters"},
	}, { // Test 1: All audio tracks get per-stream settings.
		Preset: web, Streams: Streams{AudioTracks: []int{AllTracks}},
		WantHas: [][]string{{"-map", "0:1", "-map", "0:2", "-c:a:0", "aac_at", "-b:a:0", "128000",
			"-c:a:1", "aac_at", "-b:a:1", "128000", "-ac:a:1", "2"}},
	}, { // Test 2: A chosen track by position.
		Preset: web, Streams: Streams{AudioTracks: []int{1}},
		WantHas: [][]string{{"-map", "0:2", "-c:a", "aac_at"}}, WantMissing: []string{"0:1"},
	}, { // Test 3: A track past the end.
		Preset: web, Streams: Streams{AudioTracks: []int{2}}, Want: ErrNoAudio,
	}, { // Test 4: External audio adds an input and a map, planned from its probe.
		Preset: web, Streams: Streams{ExternalAudio: []string{"dub.wav"}},
		Extra: []probe.Info{{Streams: []probe.Stream{{Kind: "audio", Codec: "pcm_s16le", Channels: 1,
			SampleRate: 96000}}}},
		WantHas: [][]string{{"-i", "in.mov", "-i", "dub.wav"}, {"-map", "0:1", "-map", "1:a:0"},
			{"-c:a:1", "aac", "-b:a:1", "128000"}},
	}, { // Test 5: External audio without an audio stream.
		Preset: web, Streams: Streams{ExternalAudio: []string{"x.png"}},
		Extra: []probe.Info{{Streams: []probe.Stream{{Kind: "video"}}}}, Want: ErrNoAudio,
	}, { // Test 6: An audio container that holds one track.
		Preset:  func(t *testing.T) preset.Preset { t.Helper(); return testPreset(t, "mp3") },
		Streams: Streams{AudioTracks: []int{AllTracks}}, Want: ErrStreams,
	}, { // Test 7: Keep converts text to mov_text and skips the bitmap MP4 cannot hold.
		Preset: web, Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesKeep}},
		WantHas: [][]string{{"-map", "0:3", "-c:s:0", "mov_text"}}, WantMissing: []string{"0:4"},
	}, { // Test 8: Keep in MKV copies text, skips the bitmap, adds the file.
		Preset:  func(t *testing.T) preset.Preset { t.Helper(); return streamsTarget(t, "mkv", "") },
		Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesKeep, External: "en.ass"}},
		WantHas: [][]string{{"-i", "en.ass"}, {"-map", "0:3", "-map", "1:s:0", "-c:s:0", "copy",
			"-c:s:1", "copy"}},
		WantMissing: []string{"0:4"},
	}, { // Test 9: Keep in a container without subtitles carries none.
		Preset:      func(t *testing.T) preset.Preset { t.Helper(); return streamsTarget(t, "avi", "mpeg4") },
		Streams:     Streams{Subtitles: Subtitles{Mode: SubtitlesKeep}},
		WantMissing: []string{"-c:s:0"},
	}, { // Test 10: Pick the bitmap track in MP4 is refused.
		Preset: web, Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesPick, Track: 1}}, Want: ErrStreams,
	}, { // Test 11: Pick in a container without subtitles is refused.
		Preset:  func(t *testing.T) preset.Preset { t.Helper(); return streamsTarget(t, "avi", "mpeg4") },
		Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesPick}}, Want: ErrStreams,
	}, { // Test 12: Pick an external file into WebM converts it to WebVTT.
		Preset: func(t *testing.T) preset.Preset {
			t.Helper()
			p := streamsTarget(t, "webm", "vp9")
			p.Audio.Codec = "opus"
			return p
		},
		Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesPick, External: "a.srt"}},
		WantHas: [][]string{{"-map", "1:s:0", "-c:s:0", "webvtt"}},
	}, { // Test 13: Burn a text track after the scale filter.
		Preset: func(t *testing.T) preset.Preset {
			t.Helper()
			p := web(t)
			p.Video.MaxWidth, p.Video.MaxHeight = 1280, 720
			return p
		},
		Streams:     Streams{Subtitles: Subtitles{Mode: SubtitlesBurn}},
		WantHas:     [][]string{{"-vf", "scale=1280:720,subtitles=filename=in.mov:si=0"}},
		WantFilters: []string{"subtitles"},
	}, { // Test 14: Burn a bitmap track through overlay and map the graph output.
		Preset: web, Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesBurn, Track: 1}},
		WantHas:     [][]string{{"-filter_complex", "[0:0][0:4]overlay=eof_action=pass[v]", "-map", "[v]"}},
		WantMissing: []string{"-vf", "0:0"}, WantFilters: []string{"overlay"},
	}, { // Test 15: Burn an external file with an awkward name.
		Preset:      web,
		Streams:     Streams{Subtitles: Subtitles{Mode: SubtitlesBurn, External: "/x/it's a:b,c.srt"}},
		WantHas:     [][]string{{"-vf", `subtitles=filename=/x/it\\\'s a\\:b\,c.srt`}},
		WantFilters: []string{"subtitles"},
	}, { // Test 16: Burn with no subtitle stream.
		Preset: web, Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesBurn}},
		Info: func(i probe.Info) probe.Info { i.Streams = i.Streams[:3]; return i }, Want: ErrNoSubtitle,
	}, { // Test 17: Burn with copied video.
		Preset:  func(t *testing.T) preset.Preset { t.Helper(); return testPreset(t, "remux") },
		Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesBurn}}, Want: ErrStreams,
	}, { // Test 18: Drop chapters and metadata, cap threads.
		Preset: web, Streams: Streams{DropChapters: true, DropMetadata: true, Threads: 4},
		WantHas:     [][]string{{"-threads", "4", "-map_chapters", "-1", "-map_metadata", "-1"}},
		WantMissing: []string{"0"},
	}, { // Test 19: Two-pass carries threads and burn-in on both passes, inputs on the last only.
		Preset: func(t *testing.T) preset.Preset {
			t.Helper()
			return testPreset(t, preset.TargetSizeID).WithTargetSize(50_000_000)
		},
		Streams:    Streams{Threads: 2, ExternalAudio: []string{"b.m4a"}},
		Info:       func(i probe.Info) probe.Info { i.Duration = 90 * time.Second; return i },
		WantHas:    [][]string{{"-threads", "2", "-an"}, {"-i", "b.m4a"}},
		WantPasses: 2,
	}, { // Test 20: Copied video keeps soft subtitles.
		Preset:  func(t *testing.T) preset.Preset { t.Helper(); return testPreset(t, "remux") },
		Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesPick}},
		WantHas: [][]string{{"-c:v", "copy"}, {"-map", "0:3", "-c:s:0", "mov_text"}},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			info := streamsInfo()
			if test.Info != nil {
				info = test.Info(info)
			}
			p := test.Preset(t)
			job := Job{Input: "in.mov", Preset: p, Streams: test.Streams}
			plan, err := Build(job, info, streamsAvail(), "out", "passlog", test.Extra...)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if err != nil {
				return
			}
			if test.WantPasses != 0 && len(plan.Passes) != test.WantPasses {
				t.Fatalf("passes = %d, want %d", len(plan.Passes), test.WantPasses)
			}
			all := strings.Join(slices.Concat(plan.Passes...), " ")
			for _, want := range test.WantHas {
				if !containsRun(plan.Passes, want) {
					t.Errorf("no pass holds %q in order:\n%s", want, all)
				}
			}
			last := plan.Passes[len(plan.Passes)-1]
			for _, missing := range test.WantMissing {
				if slices.Contains(last, missing) {
					t.Errorf("final pass holds %q:\n%s", missing, strings.Join(last, " "))
				}
			}
			if diff := cmp.Diff(test.WantFilters, plan.Filters); diff != "" {
				t.Errorf("filters mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// containsRun reports whether some pass holds want as a contiguous run.
func containsRun(passes [][]string, want []string) bool {
	for _, args := range passes {
		for i := 0; i+len(want) <= len(args); i++ {
			if slices.Equal(args[i:i+len(want)], want) {
				return true
			}
		}
	}
	return false
}

func TestPerStream(t *testing.T) {
	t.Parallel()
	got := perStream([]string{"-c:a", "flac", "-ac", "2", "-sample_fmt", "s16"}, "a", 3)
	want := []string{"-c:a:3", "flac", "-ac:a:3", "2", "-sample_fmt:a:3", "s16"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestParseFilters(t *testing.T) {
	t.Parallel()
	out := []byte("Filters:\n  T.. = Timeline support\n  A = Audio input/output\n" +
		" TS overlay           VV->V      Overlay a video source on top of the input.\n" +
		" .. anull             A->A       Pass the source unchanged to the output.\n")
	got := ParseFilters(out)
	if diff := cmp.Diff(FilterSet{"overlay": true, "anull": true}, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// streamsFixture writes a 2 second MKV with video, two audio tracks, a text subtitle, and two
// chapters, plus a WAV for external audio, and returns both paths.
func streamsFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	srt := filepath.Join(dir, "en.srt")
	meta := filepath.Join(dir, "meta.txt")
	src := filepath.Join(dir, "in.mkv")
	wav := filepath.Join(dir, "dub.wav")
	files := map[string]string{
		srt: "1\n00:00:00,000 --> 00:00:01,900\nHello\n",
		meta: ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\ntitle=A\n" +
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=1000\nEND=2000\ntitle=B\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		t.Helper()
		args = append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
		if out, err := exec.CommandContext(t.Context(), "ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("fixture: %v: %s", err, out)
		}
	}
	run("-f", "lavfi", "-i", "testsrc=size=640x360:rate=25:d=2", "-f", "lavfi", "-i", "sine=f=440:d=2",
		"-f", "lavfi", "-i", "sine=f=880:d=2", "-i", srt, "-i", meta,
		"-map", "0", "-map", "1", "-map", "2", "-map", "3", "-map_chapters", "4",
		"-c:v", "libx264", "-c:a", "aac", "-c:s", "srt", src)
	run("-f", "lavfi", "-i", "sine=f=220:d=2", wav)
	return src, wav
}

// TestEncodeStreams runs real encodes of the stream features and probes the results.
func TestEncodeStreams(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	if os.Getenv(sweepEnv) == "" {
		t.Skip("set " + sweepEnv + "=1 to run real encodes")
	}
	src, wav := streamsFixture(t)
	ff := NewFFmpeg()
	filters, err := ff.Filters(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	mkv := testPreset(t, "web")
	mkv.Container = "mkv"
	tests := []struct {
		Want         error
		Preset       preset.Preset
		Streams      Streams
		WantAudio    int
		WantSubs     []string
		WantChapters int
	}{{ // Test 0: Every audio track and text subtitles into MP4.
		Preset: testPreset(t, "web"), WantAudio: 2, WantSubs: []string{"mov_text"}, WantChapters: 2,
		Streams: Streams{AudioTracks: []int{AllTracks}, Subtitles: Subtitles{Mode: SubtitlesKeep}},
	}, { // Test 1: External audio into MKV, chapters and metadata dropped.
		Preset: mkv, WantAudio: 2, WantSubs: []string{"subrip"},
		Streams: Streams{ExternalAudio: []string{wav}, DropChapters: true, DropMetadata: true,
			Subtitles: Subtitles{Mode: SubtitlesPick}, Threads: 2},
	}, { // Test 2: Text burn-in, refused when this ffmpeg lacks libass.
		Preset: testPreset(t, "web"), WantAudio: 1, WantChapters: 2,
		Streams: Streams{Subtitles: Subtitles{Mode: SubtitlesBurn}},
		Want:    map[bool]error{true: nil, false: ErrMissingFilter}[filters.Has("subtitles")],
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			out := filepath.Join(t.TempDir(), "out."+test.Preset.Ext())
			job := Job{Input: src, Output: out, Preset: test.Preset, Streams: test.Streams}
			if err := ff.Encode(t.Context(), job, nil); !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if test.Want != nil {
				return
			}
			info, err := probe.File(t.Context(), out)
			if err != nil {
				t.Fatal(err)
			}
			var subs []string
			for _, s := range info.Kind("subtitle") {
				subs = append(subs, s.Codec)
			}
			got := []any{len(info.Kind("audio")), subs, len(info.Chapters)}
			want := []any{test.WantAudio, test.WantSubs, test.WantChapters}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("audio, subtitles, chapters mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
