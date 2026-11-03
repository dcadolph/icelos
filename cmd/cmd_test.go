package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/cmd/config"
	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/queue"
	"github.com/dcadolph/icelos/internal/serve"
)

// testFFprobe prints a fixed two-stream probe result.
const testFFprobe = `#!/bin/sh
chapters=''
case "$*" in *chapters*) chapters=',"chapters":[{"start_time":"0.0","end_time":"5.0","tags":{"title":"One"}},` +
	`{"start_time":"5.0","end_time":"10.0","tags":{"title":"Two"}}]';; esac
echo '{"format":{"format_name":"mov","duration":"10.0","size":"100"},"streams":[` +
	`{"index":0,"codec_type":"video","codec_name":"h264","width":640,"height":360},` +
	`{"index":1,"codec_type":"audio","codec_name":"aac","channels":2}]'"$chapters"'}'
`

// testFFmpeg lists encoders, fails inputs named fail, and writes the output otherwise.
const testFFmpeg = `#!/bin/sh
for last; do :; done
case "$*" in *-encoders*)
	printf ' ------\n V....D libx264 x\n V....D h264_videotoolbox x\n A....D aac x\n'; exit 0;;
esac
case "$*" in *fail.mov*) echo "broken input" >&2; exit 1;; esac
printf 'out_time_us=10000000\nspeed=4x\nprogress=end\n'
echo encoded > "$last"
`

// testTools writes the fake tools and returns their paths.
func testTools(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	ffmpeg, ffprobe := filepath.Join(dir, "ffmpeg"), filepath.Join(dir, "ffprobe")
	for path, body := range map[string]string{ffmpeg: testFFmpeg, ffprobe: testFFprobe} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil { //nolint:gosec // Test script.
			t.Fatal(err)
		}
	}
	return ffmpeg, ffprobe
}

// testRun runs the root command with args and stdin and returns stdout and the error.
func testRun(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)
	ffmpeg, ffprobe := testTools(t)
	cmd := rootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append([]string{"--ffmpeg", ffmpeg, "--ffprobe", ffprobe}, args...))
	err := cmd.ExecuteContext(t.Context())
	return out.String(), err
}

// testInputs creates empty input files and returns their paths.
func testInputs(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func TestConvert(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	in := testInputs(t, "a.mov", "b.mov", "fail.mov")
	tests := []struct {
		Want       error
		Args       []string
		WantStates []queue.State
	}{{ // Test 0: Two inputs convert.
		Args: []string{"convert", "--preset", "web", in[0], in[1]}, WantStates: []queue.State{"done", "done"},
	}, { // Test 1: A failed job sets the exit error and still reports every job.
		Args: []string{"convert", "--hardware=false", "--preset", "web", in[0], in[2]}, Want: errJobsFailed,
		WantStates: []queue.State{"done", "failed"},
	}, { // Test 2: Target size with a size.
		Args: []string{"convert", "--preset", "target-size", "--size", "2MB", in[0]}, WantStates: []queue.State{"done"},
	}, { // Test 3: Target size without a size.
		Args: []string{"convert", "--preset", "target-size", in[0]}, Want: errUsage,
	}, { // Test 4: Size on an audio preset.
		Args: []string{"convert", "--preset", "audio", "--size", "2MB", in[0]}, Want: errUsage,
	}, { // Test 5: Bad size.
		Args: []string{"convert", "--preset", "web", "--size", "huge", in[0]}, Want: preset.ErrSize,
	}, { // Test 6: Unknown preset.
		Args: []string{"convert", "--preset", "betamax", in[0]}, Want: preset.ErrUnknown,
	}, { // Test 7: Output with two inputs.
		Args: []string{"convert", "--output", "x.mp4", in[0], in[1]}, Want: errUsage,
	}, { // Test 8: Missing input.
		Args: []string{"convert", in[0] + ".nope"}, Want: queue.ErrInput,
	}, { // Test 9: No inputs.
		Args: []string{"convert"}, Want: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			out, err := testRun(t, "", append(test.Args, "--out-dir", t.TempDir())...)
			if testNum == 9 {
				if err == nil {
					t.Error("expected an argument error")
				}
				return
			}
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if test.WantStates == nil {
				return
			}
			var got []queue.Status
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("stdout %q: %v", out, err)
			}
			var states []queue.State
			for _, st := range got {
				states = append(states, st.State)
			}
			if diff := cmp.Diff(test.WantStates, states); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPresets(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	out, err := testRun(t, "", "presets", "--pretty")
	if err != nil {
		t.Fatal(err)
	}
	var got []preset.Preset
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(preset.All(), got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if !strings.Contains(out, "\n  ") {
		t.Error("--pretty output is not indented")
	}
}

func TestProbeAndEncoders(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	in := testInputs(t, "a.mov")
	out, err := testRun(t, "", "probe", in[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"durationNs":10000000000`) {
		t.Errorf("probe output = %s", out)
	}
	out, err = testRun(t, "", "encoders")
	if err != nil {
		t.Fatal(err)
	}
	var rep encoderReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	want := encoderReport{FFmpeg: rep.FFmpeg, Hardware: []string{"h264_videotoolbox"},
		Software: []string{"aac", "libx264"}, Codecs: rep.Codecs}
	if diff := cmp.Diff(want, rep); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	avail := map[string]bool{}
	for _, c := range rep.Codecs {
		avail[c.ID] = c.Available
	}
	if diff := cmp.Diff(map[string]bool{"h264": true, "aac": true, "hevc": false, "theora": false},
		map[string]bool{"h264": avail["h264"], "aac": avail["aac"], "hevc": avail["hevc"],
			"theora": avail["theora"]}); diff != "" {
		t.Errorf("codec availability mismatch (-want +got):\n%s", diff)
	}
	if len(rep.Codecs) != len(format.Codecs()) {
		t.Errorf("report has %d codecs, catalog has %d", len(rep.Codecs), len(format.Codecs()))
	}
}

func TestServe(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	t.Setenv("HOME", t.TempDir())
	out, err := testRun(t, `{"id":"1","op":"presets"}`+"\n", "serve")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var types []string
	for _, l := range lines {
		var m serve.Message
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		types = append(types, m.Type)
	}
	if diff := cmp.Diff([]string{serve.TypeReady, serve.OpPresets}, types); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestResolveTool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name       string
		Configured string
		WantPath   string
	}{{ // Test 0: Configured path wins.
		Name: "ffmpeg", Configured: "/opt/ffmpeg", WantPath: "/opt/ffmpeg",
	}, { // Test 1: No sibling falls back to PATH lookup by name.
		Name: "icelos-no-such-tool", WantPath: "icelos-no-such-tool",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := resolveTool(test.Name, test.Configured); got != test.WantPath {
				t.Errorf("resolveTool = %q, want %q", got, test.WantPath)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	t.Parallel()
	if v := resolveVersion(); v == "" {
		t.Error("empty version")
	}
}

func TestProgressPrinter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := newProgressPrinter(&buf)
	st := queue.Status{ID: "1", State: queue.StateRunning}
	st.Job.Input, st.Job.Output = "/x/clip.mov", "/x/clip.mp4"
	st.Progress.Percent, st.Progress.Speed, st.Progress.Encoder = 0.42, 2, "libx264"
	p.Print(queue.Status{ID: "1", State: queue.StateQueued})
	p.Print(st)
	p.Print(st)
	st.State = queue.StateDone
	p.Print(st)
	st.State, st.Error = queue.StateFailed, "boom"
	p.Print(st)
	want := "clip.mov:  42%  2.0x  libx264\nclip.mov: done -> /x/clip.mp4\nclip.mov: failed: boom\n"
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// testSetFlags resets the config flags and sets each key to its value.
func testSetFlags(t *testing.T, kv map[string]string) {
	t.Helper()
	config.Reset()
	t.Cleanup(config.Reset)
	for k, v := range kv {
		f := config.Lookup(k)
		if err := f.Value.Set(v); err != nil {
			t.Fatal(err)
		}
		f.Changed = true
	}
}

func TestSelectedPresetFlags(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	webm, err := format.ContainerByID("webm")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		Flags     map[string]string
		Want      error
		WantField func(preset.Preset) any
		WantValue any
	}{{ // Test 0: Container replaces a disallowed video codec.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyContainer: "webm"},
		WantField: func(p preset.Preset) any { return p.Video.Codec }, WantValue: webm.Video[0],
	}, { // Test 1: Container replaces a disallowed audio codec.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyContainer: "webm"},
		WantField: func(p preset.Preset) any { return p.Audio.Codec }, WantValue: webm.Audio[0],
	}, { // Test 2: Explicit video codec wins over the container replacement.
		Flags: map[string]string{config.KeyPreset: "web", config.KeyContainer: "mkv",
			config.KeyVideoCodec: "hevc"},
		WantField: func(p preset.Preset) any { return p.Video.Codec }, WantValue: "hevc",
	}, { // Test 3: Audio codec.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyAudioCodec: "opus"},
		WantField: func(p preset.Preset) any { return p.Audio.Codec }, WantValue: "opus",
	}, { // Test 4: Audio none clears the codec.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyAudioCodec: "none"},
		WantField: func(p preset.Preset) any { return p.Audio.Codec }, WantValue: "",
	}, { // Test 5: Resolution ID sets the fit box.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyResolution: "720p"},
		WantField: func(p preset.Preset) any { return [2]int{p.Video.MaxWidth, p.Video.MaxHeight} },
		WantValue: [2]int{1280, 720},
	}, { // Test 6: Resolution WxH sets the exact size.
		Flags: map[string]string{config.KeyPreset: "web", config.KeyResolution: "1280x720"},
		WantField: func(p preset.Preset) any {
			return [4]int{p.Video.Width, p.Video.Height, p.Video.MaxWidth, p.Video.MaxHeight}
		},
		WantValue: [4]int{1280, 720, 0, 0},
	}, { // Test 7: Resolution same clears the box.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyResolution: "same"},
		WantField: func(p preset.Preset) any { return [2]int{p.Video.MaxWidth, p.Video.MaxHeight} },
		WantValue: [2]int{0, 0},
	}, { // Test 8: Bad resolution.
		Flags: map[string]string{config.KeyPreset: "web", config.KeyResolution: "huge"}, Want: errUsage,
	}, { // Test 9: FPS.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyFPS: "29.97"},
		WantField: func(p preset.Preset) any { return p.Video.FPS }, WantValue: 29.97,
	}, { // Test 10: Bad FPS.
		Flags: map[string]string{config.KeyPreset: "web", config.KeyFPS: "fast"}, Want: errUsage,
	}, { // Test 11: Video bitrate with suffix switches quality to vbr.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyVideoBitrate: "5M"},
		WantField: func(p preset.Preset) any { return []any{p.Video.Bitrate, p.Video.RateControl} },
		WantValue: []any{5_000_000, format.RateVBR},
	}, { // Test 12: Audio bitrate with k suffix.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyAudioBitrate: "192k"},
		WantField: func(p preset.Preset) any { return p.Audio.Bitrate }, WantValue: 192_000,
	}, { // Test 13: Plain audio bitrate.
		Flags:     map[string]string{config.KeyPreset: "web", config.KeyAudioBitrate: "128000"},
		WantField: func(p preset.Preset) any { return p.Audio.Bitrate }, WantValue: 128_000,
	}, { // Test 14: Bad bitrate.
		Flags: map[string]string{config.KeyPreset: "web", config.KeyVideoBitrate: "lots"}, Want: errUsage,
	}, { // Test 15: Negative control, odd dimension is refused by Validate.
		Flags: map[string]string{config.KeyPreset: "web", config.KeyResolution: "1919x1080"},
		Want:  format.ErrSetting,
	}, { // Test 16: Unknown container is refused by Validate.
		Flags: map[string]string{config.KeyPreset: "web", config.KeyContainer: "nope"},
		Want:  preset.ErrInvalid,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			testSetFlags(t, test.Flags)
			p, err := selectedPreset()
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if test.WantField == nil {
				return
			}
			if diff := cmp.Diff(test.WantValue, test.WantField(p)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSelectedPresetCustom(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	t.Setenv("HOME", t.TempDir())
	store, err := preset.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	custom, err := preset.Lookup("web")
	if err != nil {
		t.Fatal(err)
	}
	custom.ID, custom.Name = "mine", "Mine"
	if err := store.Save(custom); err != nil {
		t.Fatal(err)
	}
	testSetFlags(t, map[string]string{config.KeyPreset: "mine"})
	p, err := selectedPreset()
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "mine" {
		t.Errorf("preset = %q, want mine", p.ID)
	}
}

func TestSelectedStreams(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	tests := []struct {
		Flags      map[string]string
		Want       error
		WantStream engine.Streams
	}{{  // Test 0: No flags.
	}, { // Test 1: All tracks.
		Flags:      map[string]string{config.KeyAudioTracks: "all"},
		WantStream: engine.Streams{AudioTracks: []int{engine.AllTracks}},
	}, { // Test 2: Track list converts to zero-based positions.
		Flags:      map[string]string{config.KeyAudioTracks: "1,3"},
		WantStream: engine.Streams{AudioTracks: []int{0, 2}},
	}, { // Test 3: Track zero.
		Flags: map[string]string{config.KeyAudioTracks: "0"}, Want: errUsage,
	}, { // Test 4: Track junk.
		Flags: map[string]string{config.KeyAudioTracks: "x"}, Want: errUsage,
	}, { // Test 5: Subtitles keep.
		Flags:      map[string]string{config.KeySubtitles: "keep"},
		WantStream: engine.Streams{Subtitles: engine.Subtitles{Mode: engine.SubtitlesKeep}},
	}, { // Test 6: Subtitles none.
		Flags:      map[string]string{config.KeySubtitles: "none"},
		WantStream: engine.Streams{Subtitles: engine.Subtitles{Mode: engine.SubtitlesNone}},
	}, { // Test 7: Subtitles burn.
		Flags:      map[string]string{config.KeySubtitles: "burn"},
		WantStream: engine.Streams{Subtitles: engine.Subtitles{Mode: engine.SubtitlesBurn}},
	}, { // Test 8: Subtitle file path.
		Flags: map[string]string{config.KeySubtitles: "/tmp/a.srt"},
		WantStream: engine.Streams{Subtitles: engine.Subtitles{Mode: engine.SubtitlesKeep,
			External: "/tmp/a.srt"}},
	}, { // Test 9: Threads.
		Flags: map[string]string{config.KeyThreads: "4"}, WantStream: engine.Streams{Threads: 4},
	}, { // Test 10: Negative threads.
		Flags: map[string]string{config.KeyThreads: "-1"}, Want: errUsage,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			testSetFlags(t, test.Flags)
			got, err := selectedStreams()
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantStream, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestJobFlags(t *testing.T) { //nolint:paralleltest // Flags are process-wide.
	in := testInputs(t, "chapters.mov")
	t.Setenv("HOME", t.TempDir())
	out, err := testRun(t, "", "convert", "--preset", "web", "--split-chapters", "--passthrough",
		in[0], "--out-dir", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var got []queue.Status
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || !got[0].Job.Passthrough {
		t.Errorf("passthrough not set on job: %+v", got)
	}
	var names []string
	for _, st := range got {
		names = append(names, filepath.Base(st.Job.Output))
	}
	if diff := cmp.Diff([]string{"chapters - 01 One.mp4", "chapters - 02 Two.mp4"}, names); diff != "" {
		t.Errorf("split chapters mismatch (-want +got):\n%s", diff)
	}
}
