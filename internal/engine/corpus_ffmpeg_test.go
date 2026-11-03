package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// corpusDir holds the real-world clips and their manifest.
const corpusDir = "testdata/media"

// corpusTargetSize is the byte limit the target-size preset gets for every clip.
const corpusTargetSize = 200_000

// Corpus check errors.
var (
	errCorpusHash     = errors.New("corpus file hash mismatch")
	errCorpusUnlisted = errors.New("corpus file not in manifest")
	errCorpusMissing  = errors.New("manifest file missing")
	errCorpusCredit   = errors.New("manifest entry lacks license or credit")
)

// corpusManifest is the parsed testdata/media/manifest.json.
type corpusManifest struct {
	// Files lists every clip in the corpus.
	Files []corpusEntry `json:"files"`
}

// corpusEntry describes one clip and where it came from.
type corpusEntry struct {
	// File is the clip's name inside the corpus directory.
	File string `json:"file"`
	// SHA256 is the hex digest the clip must match.
	SHA256 string `json:"sha256"`
	// SourceURL is where the clip was cut from.
	SourceURL string `json:"source_url"`
	// License names the license the clip is used under.
	License string `json:"license"`
	// Attribution is the credit the license requires.
	Attribution string `json:"attribution"`
}

// readCorpus parses the manifest in dir.
func readCorpus(dir string) (corpusManifest, error) {
	var m corpusManifest
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	return m, nil
}

// checkCorpus verifies that dir holds exactly the manifest's clips, unchanged and credited.
func checkCorpus(dir string) error {
	m, err := readCorpus(dir)
	if err != nil {
		return err
	}
	listed := make(map[string]bool, len(m.Files))
	for _, e := range m.Files {
		listed[e.File] = true
		if e.License == "" || e.Attribution == "" || e.SourceURL == "" {
			return fmt.Errorf("%w: %s", errCorpusCredit, e.File)
		}
		data, err := os.ReadFile(filepath.Join(dir, e.File))
		if err != nil {
			return fmt.Errorf("%w: %s", errCorpusMissing, e.File)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != e.SHA256 {
			return fmt.Errorf("%w: %s", errCorpusHash, e.File)
		}
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read corpus: %w", err)
	}
	for _, n := range names {
		if n.Name() != "manifest.json" && !listed[n.Name()] {
			return fmt.Errorf("%w: %s", errCorpusUnlisted, n.Name())
		}
	}
	return nil
}

// copyCorpus copies the corpus into a temporary directory and applies change to it.
func copyCorpus(t *testing.T, change func(dir string)) string {
	t.Helper()
	dir := t.TempDir()
	names, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(corpusDir, n.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n.Name()), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	change(dir)
	return dir
}

func TestCorpusManifest(t *testing.T) {
	t.Parallel()
	first := func(dir string) string {
		m, err := readCorpus(dir)
		if err != nil || len(m.Files) == 0 {
			t.Fatalf("empty corpus: %v", err)
		}
		return filepath.Join(dir, m.Files[0].File)
	}
	tests := []struct {
		Change func(t *testing.T, dir string)
		Want   error
	}{{ // Test 0: The committed corpus matches its manifest.
		Change: func(*testing.T, string) {}, Want: nil,
	}, { // Test 1: A changed byte fails the hash.
		Change: func(t *testing.T, dir string) {
			t.Helper()
			path := first(dir)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data[len(data)/2] ^= 0xff
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		Want: errCorpusHash,
	}, { // Test 2: A file outside the manifest is refused.
		Change: func(t *testing.T, dir string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(dir, "stray.mp4"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		Want: errCorpusUnlisted,
	}, { // Test 3: A listed file that is gone is refused.
		Change: func(t *testing.T, dir string) {
			t.Helper()
			if err := os.Remove(first(dir)); err != nil {
				t.Fatal(err)
			}
		},
		Want: errCorpusMissing,
	}, { // Test 4: An entry without a license is refused.
		Change: func(t *testing.T, dir string) {
			t.Helper()
			path := filepath.Join(dir, "manifest.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.Replace(data, []byte(`"license": "CC BY 3.0"`), []byte(`"license": ""`), 1)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		Want: errCorpusCredit,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			dir := corpusDir
			if testNum > 0 {
				dir = copyCorpus(t, func(d string) { test.Change(t, d) })
			}
			if err := checkCorpus(dir); !errors.Is(err, test.Want) {
				t.Errorf("checkCorpus() = %v, want %v", err, test.Want)
			}
		})
	}
}

// corpusOutcome is what one clip and preset produce, recorded in the golden file.
type corpusOutcome struct {
	// Error is the sentinel error name when the engine refuses the job, else empty.
	Error string `json:"error,omitempty"`
	// Streams lists the output's stream kinds and codecs in order, such as "video:h264".
	Streams []string `json:"streams,omitempty"`
	// Display is the output's width and height as shown, after rotation.
	Display string `json:"display,omitempty"`
	// Duration is the output's length rounded to a tenth of a second.
	Duration string `json:"duration,omitempty"`
	// Chapters counts the output's chapters.
	Chapters int `json:"chapters,omitempty"`
	// Copied lists the stream kinds the plan copied instead of encoding.
	Copied []string `json:"copied,omitempty"`
}

// corpusCase is one clip run through one preset.
type corpusCase struct {
	// Name is the subtest and golden key.
	Name string
	// Clip is the clip's file name in the corpus.
	Clip string
	// Preset is the conversion target.
	Preset preset.Preset
	// Passthrough copies streams that already match the preset.
	Passthrough bool
}

// ssimFloor is the lowest SSIM each preset may score against its source. Measured on the corpus
// with headroom, so a real quality regression fails while encoder jitter does not.
var ssimFloor = map[string]float64{ //nolint:gochecknoglobals // Not modified, simplifies testing.
	"apple": 0.95, "compatible": 0.95, "web": 0.93, "tiny": 0.88, preset.TargetSizeID: 0.85,
}

// corpusCases pairs every clip with every built-in preset, plus passthrough runs for the
// presets that can copy.
func corpusCases(t *testing.T) []corpusCase {
	t.Helper()
	m, err := readCorpus(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	var cases []corpusCase
	for _, e := range m.Files {
		for _, p := range preset.All() {
			if p.ID == preset.TargetSizeID {
				p = p.WithTargetSize(corpusTargetSize)
			}
			cases = append(cases, corpusCase{Name: e.File + "/" + p.ID, Clip: e.File, Preset: p})
			if p.ID == "web" || p.ID == "apple" {
				cases = append(cases, corpusCase{
					Name: e.File + "/" + p.ID + "+passthrough", Clip: e.File, Preset: p, Passthrough: true,
				})
			}
		}
	}
	return cases
}

// errorName returns the engine sentinel err wraps, so golden files stay stable across messages.
func errorName(err error) string {
	named := []struct {
		err  error
		name string
	}{
		{ErrNoVideo, "ErrNoVideo"}, {ErrNoAudio, "ErrNoAudio"}, {ErrStreams, "ErrStreams"},
		{ErrMissingEncoder, "ErrMissingEncoder"}, {ErrTargetTooSmall, "ErrTargetTooSmall"},
		{ErrNoDuration, "ErrNoDuration"}, {ErrMissingFilter, "ErrMissingFilter"},
		{format.ErrSetting, "format.ErrSetting"}, {format.ErrMode, "format.ErrMode"},
		{format.ErrUnknownCodec, "format.ErrUnknownCodec"}, {ErrEncode, "ErrEncode"},
		{ErrCopy, "ErrCopy"}, {ErrTargetMissed, "ErrTargetMissed"},
	}
	for _, n := range named {
		if errors.Is(err, n.err) {
			return n.name
		}
	}
	return "unexpected: " + err.Error()
}

// outcomeOf summarizes the probed output.
func outcomeOf(info probe.Info, plan Copy) corpusOutcome {
	o := corpusOutcome{
		Duration: strconv.FormatFloat(math.Round(info.Duration.Seconds()*10)/10, 'f', 1, 64),
		Chapters: len(info.Chapters),
	}
	for _, s := range info.Streams {
		o.Streams = append(o.Streams, s.Kind+":"+s.Codec)
	}
	if v, ok := info.FirstVideo(); ok {
		w, h := v.DisplaySize()
		o.Display = fmt.Sprintf("%dx%d", w, h)
	}
	if plan.Video != nil && plan.Video.Copy {
		o.Copied = append(o.Copied, "video")
	}
	if plan.Audio != nil && plan.Audio.Copy {
		o.Copied = append(o.Copied, "audio")
	}
	return o
}

// ffmpegOut runs ffmpeg with args and returns its standard error.
func ffmpegOut(ctx context.Context, args ...string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// cleanDecode decodes every stream of path and fails on any decoder error.
func cleanDecode(ctx context.Context, path string) error {
	out, err := ffmpegOut(ctx, "-hide_banner", "-nostdin", "-v", "error", "-xerror", "-i", path,
		"-map", "0:v?", "-map", "0:a?", "-f", "null", "-")
	if err != nil || strings.TrimSpace(out) != "" {
		return fmt.Errorf("decode %s: %v: %s", filepath.Base(path), err, out)
	}
	return nil
}

// ssimAll matches the overall score in the ssim filter's summary line.
var ssimAll = regexp.MustCompile(`SSIM .* All:([0-9.]+)`)

// ssim scores the first video stream of out against src, scaled to out's display size.
func ssim(ctx context.Context, src, out string, w, h int) (float64, error) {
	graph := fmt.Sprintf("[1:v]scale=%d:%d:flags=bicubic,format=yuv420p[ref];"+
		"[0:v]scale=%d:%d,format=yuv420p[dst];[dst][ref]ssim", w, h, w, h)
	log, err := ffmpegOut(ctx, "-hide_banner", "-nostdin", "-i", out, "-i", src,
		"-filter_complex", graph, "-f", "null", "-")
	if err != nil {
		return 0, fmt.Errorf("ssim: %w: %s", err, lastLines([]byte(log)))
	}
	m := ssimAll.FindStringSubmatch(log)
	if m == nil {
		return 0, fmt.Errorf("ssim: no score: %s", lastLines([]byte(log)))
	}
	return strconv.ParseFloat(m[1], 64)
}

// streamHash returns ffmpeg's packet hash of the first stream of kind ("v" or "a") in path.
func streamHash(ctx context.Context, path, kind string) (string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-v", "error", "-i", path,
		"-map", "0:"+kind+":0", "-c", "copy", "-f", "streamhash", "-hash", "sha256", "-")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("stream hash: %w", err)
	}
	_, sum, _ := strings.Cut(strings.TrimSpace(string(out)), "SHA256=")
	return sum, nil
}

// frameHash returns the decoded frame hashes of the first stream of kind ("v" or "a") in path,
// one per line with timestamps left out. It proves a copy whose packets were split, as unpacking
// B-frames does, still decodes to the source's frames.
func frameHash(ctx context.Context, path, kind string) (string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-v", "error", "-i", path,
		"-map", "0:"+kind+":0", "-f", "framemd5", "-")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("frame hash: %w", err)
	}
	var sums []string
	for line := range strings.Lines(string(out)) {
		if fields := strings.Split(line, ","); !strings.HasPrefix(line, "#") && len(fields) == 6 {
			sums = append(sums, strings.TrimSpace(fields[5]))
		}
	}
	if len(sums) == 0 {
		return "", fmt.Errorf("frame hash: no frames: %s", path)
	}
	return strings.Join(sums, "\n"), nil
}

// TestCorpusConvert runs every corpus clip through every built-in preset with the real ffmpeg,
// proves each output, and compares the outcomes to testdata/golden/corpus.json.
func TestCorpusConvert(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	if testing.Short() {
		t.Skip("short mode")
	}
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	ctx := context.Background()
	f := NewFFmpeg()
	avail, err := f.Encoders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cases := corpusCases(t)
	want := readCorpusGolden(t, cases)
	var mu sync.Mutex
	got := make(map[string]corpusOutcome, len(cases))
	t.Cleanup(func() { writeCorpusGolden(t, got, len(cases)) })
	dir := t.TempDir()
	for i, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			record := func(o corpusOutcome) {
				mu.Lock()
				got[test.Name] = o
				mu.Unlock()
				if diff := cmp.Diff(want[test.Name], o); !*update && diff != "" {
					t.Errorf("outcome mismatch with testdata/golden/corpus.json (-want +got):\n%s", diff)
				}
			}
			src := filepath.Join(corpusDir, test.Clip)
			srcInfo, err := probe.File(ctx, src)
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(dir, fmt.Sprintf("out%d.%s", i, test.Preset.Ext()))
			job := Job{Input: src, Output: dest, Preset: test.Preset, Passthrough: test.Passthrough}
			plan, err := Build(job, srcInfo, avail, dest, filepath.Join(dir, fmt.Sprintf("pass%d", i)))
			if err == nil {
				err = f.Encode(ctx, job, nil)
			}
			if err != nil {
				record(corpusOutcome{Error: errorName(err)})
				return
			}
			info, err := probe.File(ctx, dest)
			if err != nil {
				t.Fatalf("probe output: %v", err)
			}
			record(outcomeOf(info, plan.Copied))
			if err := cleanDecode(ctx, dest); err != nil {
				t.Error(err)
			}
			srcVideo, srcHasVideo := srcInfo.FirstVideo()
			outVideo, outHasVideo := info.FirstVideo()
			if test.Preset.HasVideo() && srcHasVideo && outHasVideo {
				sw, sh := srcVideo.DisplaySize()
				ow, oh := outVideo.DisplaySize()
				if diff := math.Abs(float64(sw)/float64(sh) - float64(ow)/float64(oh)); diff > 0.02 {
					t.Errorf("display aspect %dx%d, source %dx%d", ow, oh, sw, sh)
				}
				if plan.Copied.Video != nil && plan.Copied.Video.Copy {
					hash := streamHash
					if packedBFrames(srcInfo.Format, srcVideo) {
						hash = frameHash
					}
					want, err := hash(ctx, src, "v")
					if err != nil {
						t.Fatal(err)
					}
					if have, err := hash(ctx, dest, "v"); err != nil || have != want {
						t.Errorf("copied video differs from the source: %v", err)
					}
				} else if floor, ok := ssimFloor[test.Preset.ID]; ok {
					score, err := ssim(ctx, src, dest, ow, oh)
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("ssim %.4f", score)
					if score < floor {
						t.Errorf("ssim %.4f below the %s floor %.3f", score, test.Preset.ID, floor)
					}
				}
			}
			if test.Preset.ID == preset.TargetSizeID && info.Size > corpusTargetSize {
				t.Errorf("size %d over the %d target", info.Size, corpusTargetSize)
			}
			kind := "a"
			if test.Preset.HasVideo() {
				kind = "v"
			}
			want, err := streamDuration(ctx, src, kind)
			if err != nil {
				t.Fatal(err)
			}
			if d := (info.Duration - want).Abs(); d > 150*time.Millisecond {
				t.Errorf("duration %s, source %s stream %s", info.Duration, kind, want)
			}
		})
	}
}

// streamDuration returns how long the first stream of kind ("v" or "a") in path plays. A
// container can outlast its streams, such as when a subtitle runs past the last frame, so the
// output is held to the stream it was made from rather than to the container.
func streamDuration(ctx context.Context, path, kind string) (time.Duration, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", kind+":0",
		"-show_entries", "stream=duration:stream_tags=DURATION", "-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		return 0, fmt.Errorf("stream duration: %w", err)
	}
	for _, line := range strings.Fields(string(out)) {
		if secs, err := strconv.ParseFloat(line, 64); err == nil {
			return time.Duration(secs * float64(time.Second)), nil
		}
		var h, m int
		var secs float64
		if _, err := fmt.Sscanf(line, "%d:%d:%f", &h, &m, &secs); err == nil {
			return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
				time.Duration(secs*float64(time.Second)), nil
		}
	}
	return 0, fmt.Errorf("stream duration: none for %s in %s", kind, filepath.Base(path))
}

// corpusGolden is the outcome file for TestCorpusConvert.
const corpusGolden = "testdata/golden/corpus.json"

// readCorpusGolden loads the expected outcomes and fails when they and cases name different
// runs, so a clip or preset added without a golden entry cannot pass silently. It returns nil
// under -update.
func readCorpusGolden(t *testing.T, cases []corpusCase) map[string]corpusOutcome {
	t.Helper()
	if *update {
		return nil
	}
	data, err := os.ReadFile(corpusGolden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want map[string]corpusOutcome
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	names := make(map[string]bool, len(cases))
	for _, c := range cases {
		names[c.Name] = true
		if _, ok := want[c.Name]; !ok {
			t.Errorf("golden lacks %s, rerun with -update", c.Name)
		}
	}
	for name := range want {
		if !names[name] {
			t.Errorf("golden has stale %s, rerun with -update", name)
		}
	}
	return want
}

// writeCorpusGolden rewrites the outcome file under -update once every case has run.
func writeCorpusGolden(t *testing.T, got map[string]corpusOutcome, total int) {
	t.Helper()
	if !*update {
		return
	}
	if len(got) != total {
		t.Fatalf("-update needs the whole matrix: ran %d of %d cases", len(got), total)
	}
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corpusGolden, append(data, '\n'), 0o644); err != nil { //nolint:gosec // Golden file.
		t.Fatal(err)
	}
}
