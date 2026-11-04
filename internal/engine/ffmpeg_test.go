package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
	"github.com/dcadolph/icelos/internal/util/tooltest"
)

// fakeFFmpegScript is a shell stand-in for ffmpeg. MODE selects its behavior.
const fakeFFmpegScript = `#!/bin/sh
MODE=%q
for last; do :; done
case "$*" in *-encoders*)
	[ "$MODE" = noencoders ] && exit 1
	printf ' V..... = Video\n ------\n V....D libx264 x\n V....D libx265 x\n'
	printf ' V....D h264_videotoolbox x\n V....D hevc_videotoolbox x\n A....D aac x\n'
	case "$MODE" in trial*) printf ' V....D h264_nvenc x\n V....D hevc_qsv x\n';; esac
	exit 0;;
esac
case "$*" in *lavfi*)
	[ "$MODE" = trialfail ] && { echo "Cannot load nvcuda.dll" >&2; exit 1; }
	[ "$MODE" = trialhang ] && { sleep 30; exit 0; }
	case "$*" in *hevc_qsv*) exit 1;; esac
	exit 0;;
esac
case "$*" in *videotoolbox*)
	[ "$MODE" = hwfail ] && { echo "Error: cannot create compression session" >&2; exit 1; };;
esac
[ "$MODE" = fail ] && { echo "in.mov: Invalid data found when processing input" >&2; exit 1; }
[ "$MODE" = sleep ] && { sleep 30; exit 0; }
printf 'fps=30\nout_time_us=5000000\nspeed=2.5x\nprogress=continue\n'
printf 'fps=30\nout_time_us=10000000\nspeed=2.5x\nprogress=end\n'
[ "$last" = /dev/null ] || echo encoded > "$last"
`

// newTestFFmpeg writes a fake ffmpeg in mode and returns a driver that probes as info.
func newTestFFmpeg(t *testing.T, mode string, info probe.Info, probeErr error) *FFmpeg {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(bin, []byte(fmt.Sprintf(fakeFFmpegScript, mode)), 0o700); err != nil { //nolint:gosec // Test script.
		t.Fatal(err)
	}
	return WithFFmpeg(bin, ProberFunc(func(context.Context, string) (probe.Info, error) {
		return info, probeErr
	}))
}

// testRecorder collects progress reports safely.
type testRecorder struct {
	// mu guards got.
	mu sync.Mutex
	// got holds every report in order.
	got []Progress
}

// Report records p.
func (r *testRecorder) Report(p Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, p)
}

func TestFFmpegEncode(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	errProbe := errors.New("probe broke")
	tests := []struct {
		Want         error
		Mode         string
		Preset       string
		TargetBytes  int64
		Hardware     bool
		Overwrite    bool
		PreExisting  bool
		SameFile     bool
		ProbeErr     error
		WantOutput   bool
		WantLast     Progress
		WantHardware []bool
	}{{ // Test 0: Software encode lands the output and reports completion.
		Preset: "web", WantOutput: true,
		WantLast:     Progress{Percent: 1, Pass: 1, Passes: 1, Elapsed: 10 * time.Second, Speed: 2.5, FPS: 30, Encoder: "libx264"},
		WantHardware: []bool{false},
	}, { // Test 1: Hardware failure retries in software.
		Mode: "hwfail", Preset: "apple", Hardware: true, WantOutput: true,
		WantLast:     Progress{Percent: 1, Pass: 1, Passes: 1, Elapsed: 10 * time.Second, Speed: 2.5, FPS: 30, Encoder: "libx265"},
		WantHardware: []bool{true, false},
	}, { // Test 2: Hardware success stays on hardware.
		Preset: "apple", Hardware: true, WantOutput: true,
		WantLast:     Progress{Percent: 1, Pass: 1, Passes: 1, Elapsed: 10 * time.Second, Speed: 2.5, FPS: 30, Encoder: "hevc_videotoolbox", Hardware: true},
		WantHardware: []bool{true},
	}, { // Test 3: Two passes report the second pass last.
		Preset: preset.TargetSizeID, TargetBytes: 5_000_000, WantOutput: true,
		WantLast:     Progress{Percent: 1, Pass: 2, Passes: 2, Elapsed: 10 * time.Second, Speed: 2.5, FPS: 30, Encoder: "libx264"},
		WantHardware: []bool{false},
	}, { // Test 4: ffmpeg failure surfaces stderr and leaves nothing behind.
		Mode: "fail", Preset: "web", Want: ErrEncode,
		WantLast: Progress{Pass: 1, Passes: 1, Encoder: "libx264"}, WantHardware: []bool{false},
	}, { // Test 5: Existing output without overwrite.
		Preset: "web", PreExisting: true, Want: ErrExists,
	}, { // Test 6: Existing output with overwrite.
		Preset: "web", PreExisting: true, Overwrite: true, WantOutput: true,
		WantLast:     Progress{Percent: 1, Pass: 1, Passes: 1, Elapsed: 10 * time.Second, Speed: 2.5, FPS: 30, Encoder: "libx264"},
		WantHardware: []bool{false},
	}, { // Test 7: Output path is the input path.
		Preset: "web", SameFile: true, Overwrite: true, Want: ErrSameFile,
	}, { // Test 8: Probe failure propagates.
		Preset: "web", ProbeErr: errProbe, Want: errProbe,
	}, { // Test 9: Encoder list failure.
		Mode: "noencoders", Preset: "web", Want: ErrEncoders,
	}, { // Test 10: Invalid preset is rejected before any work.
		Preset: "remux", TargetBytes: 1, Want: preset.ErrInvalid,
	}, { // Test 11: Missing encoder from the fake list.
		Preset: "mp3", Want: ErrMissingEncoder,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			in, out := filepath.Join(dir, "in.mov"), filepath.Join(dir, "out.mp4")
			if test.SameFile {
				out = in
			}
			if test.PreExisting {
				if err := os.WriteFile(out, []byte("old"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			p := testPreset(t, test.Preset)
			if test.TargetBytes != 0 {
				p = p.WithTargetSize(test.TargetBytes)
			}
			f := newTestFFmpeg(t, test.Mode, testInfo(), test.ProbeErr)
			rec := &testRecorder{}
			job := Job{Input: in, Output: out, Preset: p, Hardware: test.Hardware, Overwrite: test.Overwrite}
			err := f.Encode(t.Context(), job, rec.Report)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if test.Mode == "fail" && !strings.Contains(err.Error(), "Invalid data found") {
				t.Errorf("error %q lacks ffmpeg stderr", err)
			}
			body, readErr := os.ReadFile(out)
			if test.WantOutput && (readErr != nil || string(body) != "encoded\n") {
				t.Errorf("output = %q, %v; want encoded file", body, readErr)
			}
			if !test.WantOutput && !test.PreExisting && readErr == nil {
				t.Errorf("unexpected output file")
			}
			if _, err := os.Stat(partialPath(out, p.Ext())); !os.IsNotExist(err) {
				t.Errorf("partial file left behind: %v", err)
			}
			var last Progress
			var hw []bool
			for _, r := range rec.got {
				if len(hw) == 0 || hw[len(hw)-1] != r.Hardware {
					hw = append(hw, r.Hardware)
				}
				last = r
			}
			if diff := cmp.Diff(test.WantLast, last); diff != "" {
				t.Errorf("last report mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantHardware, hw); diff != "" {
				t.Errorf("hardware sequence mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFFmpegProgressMonotonic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := testPreset(t, preset.TargetSizeID).WithTargetSize(5_000_000)
	f := newTestFFmpeg(t, "", testInfo(), nil)
	rec := &testRecorder{}
	job := Job{Input: filepath.Join(dir, "in.mov"), Output: filepath.Join(dir, "out.mp4"), Preset: p}
	if err := f.Encode(t.Context(), job, rec.Report); err != nil {
		t.Fatal(err)
	}
	var percents []float64
	for _, r := range rec.got {
		percents = append(percents, r.Percent)
	}
	want := []float64{0, 0.25, 0.5, 0.5, 0.75, 1}
	if diff := cmp.Diff(want, percents); diff != "" {
		t.Errorf("percent mismatch (-want +got):\n%s", diff)
	}
	if eta := rec.got[1].ETA; eta != 6*time.Second {
		t.Errorf("first ETA = %s, want 6s", eta)
	}
}

func TestFFmpegCancel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := newTestFFmpeg(t, "sleep", testInfo(), nil)
	if _, err := f.Encoders(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	out := filepath.Join(dir, "out.mp4")
	job := Job{Input: filepath.Join(dir, "in.mov"), Output: out, Preset: testPreset(t, "web")}
	start := time.Now()
	err := f.Encode(ctx, job, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want %v", err, context.DeadlineExceeded)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("cancel took %s", elapsed)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("output exists after cancel: %v", err)
	}
}

func TestFFmpegEncodersCanceled(t *testing.T) {
	t.Parallel()
	f := newTestFFmpeg(t, "", probe.Info{}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.Encoders(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want %v", err, context.Canceled)
	}
}

func TestFFmpegEncodersCached(t *testing.T) {
	t.Parallel()
	f := newTestFFmpeg(t, "", probe.Info{}, nil)
	first, err := f.Encoders(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.bin); err != nil {
		t.Fatal(err)
	}
	second, err := f.Encoders(t.Context())
	if err != nil {
		t.Fatalf("cached call ran ffmpeg again: %v", err)
	}
	if diff := cmp.Diff(first, second); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestEncodersTrial(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Mode      string
		WantNames []string
	}{{ // Test 0: A failed trial drops a listed GPU encoder.
		Mode: "trialfail", WantNames: []string{"aac", "h264_videotoolbox", "hevc_videotoolbox", "libx264",
			"libx265"},
	}, { // Test 1: A passed trial keeps it, and each encoder is tried on its own.
		Mode: "trialpass", WantNames: []string{"aac", "h264_nvenc", "h264_videotoolbox", "hevc_videotoolbox",
			"libx264", "libx265"},
	}, { // Test 2: Encoders without a trial are kept without one.
		Mode: "", WantNames: []string{"aac", "h264_videotoolbox", "hevc_videotoolbox", "libx264", "libx265"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			set, err := newTestFFmpeg(t, test.Mode, probe.Info{}, nil).Encoders(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			got := slices.Sorted(maps.Keys(set))
			if diff := cmp.Diff(test.WantNames, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEncodersTrialCanceled(t *testing.T) {
	t.Parallel()
	f := newTestFFmpeg(t, "trialhang", probe.Info{}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := f.Encoders(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want %v", err, context.DeadlineExceeded)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("canceled trial took %s", elapsed)
	}
	if f.avail != nil {
		t.Error("a canceled trial cached the encoder list")
	}
}

func TestWithFFmpegPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Bin    string
		Prober Prober
	}{{ // Test 0: Empty binary path.
		Bin: "", Prober: ProberFunc(probe.File),
	}, { // Test 1: Nil prober.
		Bin: "ffmpeg", Prober: nil,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Error("expected panic")
				}
			}()
			WithFFmpeg(test.Bin, test.Prober)
		})
	}
}

func TestEncoderFunc(t *testing.T) {
	t.Parallel()
	errSentinel := errors.New("sentinel")
	var enc Encoder = EncoderFunc(func(context.Context, Job, ReportFunc) error { return errSentinel })
	if err := enc.Encode(t.Context(), Job{}, nil); !errors.Is(err, errSentinel) {
		t.Errorf("error = %v, want %v", err, errSentinel)
	}
}

func TestTailBuffer(t *testing.T) {
	t.Parallel()
	tb := &tailBuffer{max: 5}
	for _, s := range []string{"abc", "defg", "h"} {
		if n, err := tb.Write([]byte(s)); err != nil || n != len(s) {
			t.Fatalf("Write(%q) = %d, %v", s, n, err)
		}
	}
	if got := tb.String(); got != "defgh" {
		t.Errorf("tail = %q, want %q", got, "defgh")
	}
}

// TestRealFFmpeg encodes a one second 128x96 test pattern with the installed ffmpeg.
func TestRealFFmpeg(t *testing.T) {
	t.Parallel()
	tooltest.Require(t, "ffmpeg", "ffprobe")
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mov")
	gen := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=128x96:rate=24:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("make source: %v: %s", err, out)
	}
	tests := []struct {
		Preset    string
		WantKinds []string
	}{{ // Test 0: Software H.264 with audio.
		Preset: "web", WantKinds: []string{"video", "audio"},
	}, { // Test 1: Audio only.
		Preset: "mp3", WantKinds: []string{"audio"},
	}, { // Test 2: Remux copies streams.
		Preset: "remux", WantKinds: []string{"video", "audio"},
	}}
	f := NewFFmpeg()
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			p := testPreset(t, test.Preset)
			out, err := OutputPath(src, p.Ext(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			rec := &testRecorder{}
			if err := f.Encode(t.Context(), Job{Input: src, Output: out, Preset: p}, rec.Report); err != nil {
				t.Fatal(err)
			}
			info, err := probe.File(t.Context(), out)
			if err != nil {
				t.Fatal(err)
			}
			var kinds []string
			for _, s := range info.Streams {
				kinds = append(kinds, s.Kind)
			}
			if diff := cmp.Diff(test.WantKinds, kinds); diff != "" {
				t.Errorf("streams mismatch (-want +got):\n%s", diff)
			}
			if len(rec.got) == 0 || rec.got[len(rec.got)-1].Percent != 1 {
				t.Errorf("final report not complete: %+v", rec.got)
			}
		})
	}
}
