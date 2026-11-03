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
	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/queue"
	"github.com/dcadolph/icelos/internal/serve"
)

// testFFprobe prints a fixed two-stream probe result.
const testFFprobe = `#!/bin/sh
echo '{"format":{"format_name":"mov","duration":"10.0","size":"100"},"streams":[` +
	`{"index":0,"codec_type":"video","codec_name":"h264","width":640,"height":360},` +
	`{"index":1,"codec_type":"audio","codec_name":"aac","channels":2}]}'
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
