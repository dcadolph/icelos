package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
	"github.com/dcadolph/icelos/internal/queue"
)

// testOutput is a concurrency-safe writer that decodes messages as they arrive.
type testOutput struct {
	// mu guards buf.
	mu sync.Mutex
	// buf holds everything written.
	buf bytes.Buffer
}

// Write appends p.
func (o *testOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

// Messages decodes every complete line written so far.
func (o *testOutput) Messages(t *testing.T) []Message {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []Message
	sc := bufio.NewScanner(bytes.NewReader(o.buf.Bytes()))
	for sc.Scan() {
		var m Message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad output line %q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

// testSession starts a server with a fake encoder and a store in a temporary folder, and
// returns its input and output.
func testSession(t *testing.T) (*io.PipeWriter, *testOutput, <-chan error) {
	t.Helper()
	return testSessionWith(t, preset.WithDir(t.TempDir()))
}

// testSessionWith starts a server with a fake encoder and the given preset store.
func testSessionWith(t *testing.T, store *preset.Store) (*io.PipeWriter, *testOutput, <-chan error) {
	t.Helper()
	enc := engine.EncoderFunc(func(ctx context.Context, job engine.Job, report engine.ReportFunc) error {
		report(engine.Progress{Percent: 0.5, Pass: 1, Passes: 1})
		if strings.Contains(job.Input, "block") {
			<-ctx.Done()
			return ctx.Err()
		}
		return os.WriteFile(job.Output, nil, 0o600)
	})
	prober := engine.ProberFunc(func(_ context.Context, path string) (probe.Info, error) {
		if strings.HasSuffix(path, "missing.mov") {
			return probe.Info{}, probe.ErrProbe
		}
		info := probe.Info{Format: "mov", Duration: time.Second, Streams: []probe.Stream{
			{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080, AvgFrameRate: "30/1"},
			{Index: 1, Kind: "audio", Codec: "aac", Channels: 2, SampleRate: 48000},
		}}
		if strings.HasSuffix(path, ".m4a") {
			info.Streams = info.Streams[1:]
		}
		if strings.Contains(path, "chapters") {
			info.Chapters = []probe.Chapter{{End: time.Second / 2, Title: "One"},
				{Start: time.Second / 2, End: time.Second, Title: "Two"}}
		}
		return info, nil
	})
	encoders := func(context.Context) (engine.EncoderSet, error) {
		return engine.EncoderSet{"libx264": true, "hevc_videotoolbox": true, "aac": true, "png": true}, nil
	}
	out := &testOutput{}
	srv := New(out, Config{Encoder: enc, Prober: prober, Encoders: encoders, Presets: store,
		Version: "v0.0.0-test"})
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- srv.Run(t.Context(), pr) }()
	return pw, out, done
}

// testSend writes requests as lines.
func testSend(t *testing.T, w io.Writer, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if _, err := io.WriteString(w, l+"\n"); err != nil {
			t.Fatal(err)
		}
	}
}

// waitFor polls out until match returns true for some message.
func waitFor(t *testing.T, out *testOutput, match func(Message) bool) Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range out.Messages(t) {
			if match(m) {
				return m
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no matching message in %+v", out.Messages(t))
	return Message{}
}

// byReply returns a matcher for the reply to request id.
func byReply(id string) func(Message) bool {
	return func(m Message) bool { return m.Reply == id }
}

// jobState returns a matcher for a job event in state for job id.
func jobState(id string, state queue.State) func(Message) bool {
	return func(m Message) bool {
		return m.Type == TypeJob && m.Status != nil && m.Status.ID == id && m.Status.State == state
	}
}

func TestSession(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	dir := t.TempDir()
	clip, block := filepath.Join(dir, "clip.mov"), filepath.Join(dir, "block.mov")
	for _, p := range []string{clip, block} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	in, out, done := testSession(t)
	if first := waitFor(t, out, func(Message) bool { return true }); first.Type != TypeReady || first.Version != "v0.0.0-test" {
		t.Fatalf("first message = %+v, want ready", first)
	}
	testSend(t, in,
		`{"id":"p","op":"presets"}`,
		`{"id":"c","op":"capabilities"}`,
		`{"id":"i","op":"probe","path":"`+clip+`"}`,
		`{"id":"im","op":"probe","path":"`+filepath.Join(dir, "missing.mov")+`"}`,
		`not json`,
		`{"id":"u","op":"dance"}`,
		`{"id":"a0","op":"add"}`,
		`{"id":"a1","op":"add","input":"`+clip+`","preset":"target-size"}`,
		`{"id":"a2","op":"add","input":"`+clip+`","preset":"target-size","size":"lots"}`,
		`{"id":"a3","op":"add","input":"`+clip+`","preset":"betamax"}`,
		`{"id":"a4","op":"add","input":"`+clip+`","preset":"web","hardware":false}`,
		`{"id":"a5","op":"add","input":"`+clip+`","preset":"target-size","size":"8MB"}`,
		`{"id":"x","op":"cancel","job":"99"}`,
		`{"id":"cat","op":"catalog"}`,
		`{"id":"b0","op":"add","input":"`+clip+`","presetBody":{"id":"mine","name":"Mine","container":"mkv",`+
			`"video":{"codec":"hevc","rateControl":"quality","quality":1},"audio":{"codec":"opus","bitrate":128000}}}`,
		`{"id":"b1","op":"add","input":"`+clip+`","preset":"web","presetBody":{"id":"bad","container":"mp4",`+
			`"video":{"codec":"vp8"}}}`,
	)
	tests := []struct {
		Reply     string
		WantType  string
		WantError string
	}{
		{Reply: "p", WantType: OpPresets},                                                      // Test 0: Presets.
		{Reply: "c", WantType: OpCapabilities},                                                 // Test 1: Capabilities.
		{Reply: "i", WantType: OpProbe},                                                        // Test 2: Probe.
		{Reply: "im", WantType: TypeError, WantError: "probe failed"},                          // Test 3: Probe failure.
		{Reply: "u", WantType: TypeError, WantError: `unknown op: "dance"`},                    // Test 4: Unknown op.
		{Reply: "a0", WantType: TypeError, WantError: "bad request: input required"},           // Test 5: No input.
		{Reply: "a1", WantType: TypeError, WantError: "bad request: target-size needs a size"}, // Test 6: No size.
		{Reply: "a2", WantType: TypeError, WantError: `invalid size: "lots"`},                  // Test 7: Bad size.
		{Reply: "a3", WantType: TypeError, WantError: `unknown preset: "betamax"`},             // Test 8: Bad preset.
		{Reply: "a4", WantType: OpAdd},                                                         // Test 9: Added.
		{Reply: "a5", WantType: OpAdd},                                                         // Test 10: Added with size.
		{Reply: "x", WantType: TypeError, WantError: "unknown job: 99"},                        // Test 11: Cancel unknown.
		{Reply: "cat", WantType: OpCatalog},                                                    // Test 12: Catalog.
		{Reply: "b0", WantType: OpAdd},                                                         // Test 13: Inline preset.
		{Reply: "b1", WantType: TypeError, WantError: "vp8"},                                   // Test 14: Bad inline preset.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			got := waitFor(t, out, byReply(test.Reply))
			if got.Type != test.WantType || !strings.Contains(got.Error, test.WantError) ||
				(test.WantError == "") != (got.Error == "") {
				t.Errorf("reply = %q %q, want %q containing %q", got.Type, got.Error, test.WantType, test.WantError)
			}
		})
	}
	bad := waitFor(t, out, func(m Message) bool { return m.Type == TypeError && m.Reply == "" })
	if !strings.HasPrefix(bad.Error, "bad request") {
		t.Errorf("bad json error = %q", bad.Error)
	}
	if c := waitFor(t, out, byReply("c")); !cmp.Equal(c.Encoders, []string{"aac", "hevc_videotoolbox", "libx264"}) {
		t.Errorf("encoders = %v", c.Encoders)
	}
	if p := waitFor(t, out, byReply("p")); len(p.Presets) == 0 {
		t.Error("no presets")
	}
	if c := waitFor(t, out, byReply("cat")).Catalog; c == nil || len(c.Containers) == 0 ||
		len(c.Codecs) == 0 || len(c.Domains.Resolutions) == 0 {
		t.Errorf("catalog = %+v", c)
	}
	if b := waitFor(t, out, byReply("b0")); b.Status == nil || b.Status.Job.Preset.ID != "mine" ||
		b.Status.Job.Preset.Video.Codec != "hevc" || filepath.Ext(b.Status.Job.Output) != ".mkv" {
		t.Errorf("inline preset job = %+v", b.Status)
	}
	added := waitFor(t, out, byReply("a4"))
	if added.Status.Job.Hardware || filepath.Base(added.Status.Job.Output) != "clip.mp4" {
		t.Errorf("added job = %+v", added.Status.Job)
	}
	if sized := waitFor(t, out, byReply("a5")); sized.Status.Job.Preset.TargetBytes != 8_000_000 ||
		filepath.Base(sized.Status.Job.Output) != "clip 2.mp4" {
		t.Errorf("sized job = %+v", sized.Status.Job)
	}
	waitFor(t, out, jobState("1", queue.StateDone))
	waitFor(t, out, jobState("2", queue.StateDone))
	waitFor(t, out, jobState("3", queue.StateDone))

	testSend(t, in, `{"id":"b","op":"add","input":"`+block+`","preset":"web"}`)
	waitFor(t, out, jobState("4", queue.StateRunning))
	testSend(t, in, `{"id":"l","op":"list"}`)
	if l := waitFor(t, out, byReply("l")); len(l.Jobs) != 4 {
		t.Errorf("list = %d jobs, want 4", len(l.Jobs))
	}
	testSend(t, in, `{"id":"k","op":"cancel","job":"4"}`)
	if k := waitFor(t, out, byReply("k")); k.Type != TypeOK {
		t.Errorf("cancel reply = %+v", k)
	}
	waitFor(t, out, jobState("4", queue.StateCanceled))
	testSend(t, in, `{"id":"cl","op":"clear"}`, `{"id":"l2","op":"list"}`)
	if l := waitFor(t, out, byReply("l2")); len(l.Jobs) != 0 {
		t.Errorf("after clear, list = %d jobs", len(l.Jobs))
	}
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
}

func TestPlan(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	dir := t.TempDir()
	clip, voice := filepath.Join(dir, "clip.mov"), filepath.Join(dir, "voice.m4a")
	chaptered := filepath.Join(dir, "chapters.mov")
	for _, p := range []string{clip, voice, chaptered} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	in, out, done := testSession(t)
	small := `"presetBody":{"id":"small","name":"Small","container":"mp4",` +
		`"video":{"codec":"h264","width":1280,"rateControl":"quality","quality":2},` +
		`"audio":{"codec":"aac","bitrate":128000}}`
	testSend(t, in,
		`{"id":"p0","op":"plan","input":"`+clip+`","preset":"compatible","hardware":false}`,
		`{"id":"p1","op":"plan","input":"`+clip+`",`+small+`}`,
		`{"id":"p2","op":"plan","input":"`+clip+`","preset":"tiny"}`,
		`{"id":"p3","op":"plan","input":"`+clip+`","preset":"web","streams":{"audioTracks":[-2]}}`,
		`{"id":"p4","op":"plan","input":"`+filepath.Join(dir, "missing.mov")+`","preset":"web"}`,
		`{"id":"p5","op":"plan","input":"`+clip+`","preset":"compatible","outDir":"`+dir+`",`+
			`"streams":{"externalAudio":["`+voice+`"],"threads":2}}`,
		`{"id":"p6","op":"plan","input":"`+voice+`","preset":"web"}`,
		`{"id":"p7","op":"plan","input":"`+clip+`","preset":"compatible","hardware":false,"passthrough":true}`,
		`{"id":"p8","op":"plan","input":"`+clip+`",`+small+`,"passthrough":true}`,
		`{"id":"a0","op":"add","input":"`+clip+`","preset":"web","streams":{"threads":3,"dropChapters":true}}`,
		`{"id":"a1","op":"add","input":"`+clip+`","preset":"web","streams":{"subtitles":{"mode":"wild"}}}`,
		`{"id":"s0","op":"add","input":"`+chaptered+`","preset":"web","splitChapters":true}`,
		`{"id":"s1","op":"add","input":"`+clip+`","preset":"remux","splitChapters":true}`,
		`{"id":"l","op":"list"}`,
	)
	verdict := func(v *engine.Verdict) string {
		switch {
		case v == nil:
			return "none"
		case v.Copy:
			return "copy"
		}
		return v.Reason
	}
	tests := []struct {
		Reply        string
		WantError    string
		WantVideo    string
		WantAudio    string
		WantCopied   string
		WantEncoder  string
		WantPasses   int
		WantLossless bool
		WantHardware bool
	}{
		{Reply: "p0", WantVideo: "copy", WantAudio: "copy", WantEncoder: "libx264",
			WantCopied: "passthrough off passthrough off", WantPasses: 1}, // Test 0: Copyable source.
		{Reply: "p1", WantVideo: engine.ReasonResize, WantAudio: "copy", WantEncoder: "libx264",
			WantCopied: "resize passthrough off", WantPasses: 1}, // Test 1: Resize rules video out.
		{Reply: "p2", WantVideo: engine.ReasonCodec, WantAudio: "copy", WantEncoder: "hevc_videotoolbox",
			WantCopied: "codec passthrough off", WantHardware: true, WantPasses: 1}, // Test 2: Hardware HEVC.
		{Reply: "p3", WantError: "stream selection invalid"}, // Test 3: Bad streams.
		{Reply: "p4", WantError: "probe failed"},             // Test 4: Probe failure.
		{Reply: "p5", WantVideo: "copy", WantAudio: "copy", WantEncoder: "libx264",
			WantCopied: "passthrough off passthrough off", WantPasses: 1}, // Test 5: Probed external audio.
		{Reply: "p6", WantError: "input has no video"}, // Test 6: Audio file, video preset.
		{Reply: "p7", WantVideo: "copy", WantAudio: "copy", WantCopied: "copy copy", WantLossless: true,
			WantPasses: 1}, // Test 7: Passthrough copies a copyable source.
		{Reply: "p8", WantVideo: engine.ReasonResize, WantAudio: "copy", WantEncoder: "libx264",
			WantCopied: "resize copy", WantPasses: 1}, // Test 8: Passthrough with a resize.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			got := waitFor(t, out, byReply(test.Reply))
			if test.WantError != "" {
				if got.Type != TypeError || !strings.Contains(got.Error, test.WantError) {
					t.Errorf("reply = %q %q, want error containing %q", got.Type, got.Error, test.WantError)
				}
				return
			}
			p := got.Plan
			if got.Type != OpPlan || p == nil {
				t.Fatalf("reply = %+v, want a plan", got)
			}
			if v, a := verdict(p.Copy.Video), verdict(p.Copy.Audio); v != test.WantVideo || a != test.WantAudio {
				t.Errorf("verdicts = %s %s, want %s %s", v, a, test.WantVideo, test.WantAudio)
			}
			if c := verdict(p.Copied.Video) + " " + verdict(p.Copied.Audio); c != test.WantCopied {
				t.Errorf("copied = %s, want %s", c, test.WantCopied)
			}
			if p.Lossless != test.WantLossless || p.Encoder != test.WantEncoder ||
				p.Hardware != test.WantHardware || len(p.Passes) != test.WantPasses {
				t.Errorf("plan = %+v", p)
			}
		})
	}
	if p := waitFor(t, out, byReply("p0")).Plan; p == nil || p.Output != filepath.Join(dir, "clip.mp4") ||
		!slices.Contains(p.Passes[0], clip) {
		t.Errorf("plan output or input = %+v", p)
	}
	if p := waitFor(t, out, byReply("p5")).Plan; p == nil || !slices.Contains(p.Passes[0], voice) ||
		!slices.Contains(p.Passes[0], "-threads") {
		t.Errorf("plan with streams = %+v", p)
	}
	if a := waitFor(t, out, byReply("a0")); a.Status == nil || a.Status.Job.Streams.Threads != 3 ||
		!a.Status.Job.Streams.DropChapters {
		t.Errorf("add with streams = %+v", a.Status)
	}
	if a := waitFor(t, out, byReply("a1")); a.Type != TypeError || !strings.Contains(a.Error, "subtitle mode") {
		t.Errorf("add with bad streams = %q %q", a.Type, a.Error)
	}
	s0 := waitFor(t, out, byReply("s0"))
	var names []string
	for _, st := range s0.Jobs {
		names = append(names, filepath.Base(st.Job.Output))
	}
	if s0.Status == nil || s0.Status.ID != "2" ||
		!cmp.Equal(names, []string{"chapters - 01 One.mp4", "chapters - 02 Two.mp4"}) {
		t.Errorf("split add = %+v, names %q", s0.Status, names)
	}
	if s1 := waitFor(t, out, byReply("s1")); s1.Status == nil || len(s1.Jobs) != 0 ||
		s1.Status.Job.Range != (engine.Range{}) {
		t.Errorf("split add without chapters = %+v, %d jobs", s1.Status, len(s1.Jobs))
	}
	if l := waitFor(t, out, byReply("l")); len(l.Jobs) != 4 {
		t.Errorf("list = %d jobs, want 4: plan must not queue", len(l.Jobs))
	}
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
}

func TestCloseCancelsRunning(t *testing.T) {
	t.Parallel()
	block := filepath.Join(t.TempDir(), "block.mov")
	if err := os.WriteFile(block, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	in, out, done := testSession(t)
	testSend(t, in, `{"op":"add","input":"`+block+`","preset":"web"}`)
	waitFor(t, out, jobState("1", queue.StateRunning))
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
	waitFor(t, out, jobState("1", queue.StateCanceled))
}

// testFailWriter fails every write.
type testFailWriter struct{}

// Write always fails.
func (testFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWriteError(t *testing.T) {
	t.Parallel()
	enc := engine.EncoderFunc(func(context.Context, engine.Job, engine.ReportFunc) error { return nil })
	prober := engine.ProberFunc(func(context.Context, string) (probe.Info, error) { return probe.Info{}, nil })
	srv := New(testFailWriter{}, Config{Encoder: enc, Prober: prober, Presets: preset.WithDir(t.TempDir()),
		Encoders: func(context.Context) (engine.EncoderSet, error) { return nil, nil }})
	if err := srv.Run(t.Context(), strings.NewReader(`{"op":"presets"}`)); !errors.Is(err, ErrWrite) {
		t.Errorf("error = %v, want %v", err, ErrWrite)
	}
}

func TestNewPanics(t *testing.T) {
	t.Parallel()
	enc := engine.EncoderFunc(func(context.Context, engine.Job, engine.ReportFunc) error { return nil })
	tests := []struct {
		W   io.Writer
		Cfg Config
	}{{ // Test 0: Nil writer.
		W: nil, Cfg: Config{Encoder: enc},
	}, { // Test 1: Missing prober and encoders.
		W: &bytes.Buffer{}, Cfg: Config{Encoder: enc},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Error("expected panic")
				}
			}()
			New(test.W, test.Cfg)
		})
	}
}

func TestNewCatalog(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In            engine.EncoderSet
		WantAvailable map[string]bool
	}{{ // Test 0: libx265 and libtheora make HEVC and Theora available.
		In:            engine.EncoderSet{"libx265": true, "libtheora": true, "aac": true},
		WantAvailable: map[string]bool{"hevc": true, "theora": true, "aac": true, "h264": false},
	}, { // Test 1: Without libx265 and libtheora HEVC and Theora are unavailable.
		In:            engine.EncoderSet{"aac": true},
		WantAvailable: map[string]bool{"hevc": false, "theora": false, "aac": true, "h264": false},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c := NewCatalog(test.In)
			got := map[string]bool{}
			for _, codec := range c.Codecs {
				if _, ok := test.WantAvailable[codec.ID]; ok {
					got[codec.ID] = codec.Available
				}
			}
			if diff := cmp.Diff(test.WantAvailable, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCatalogFitsOneLine(t *testing.T) {
	t.Parallel()
	all := engine.EncoderSet{}
	for _, c := range format.Codecs() {
		for _, e := range c.Encoders {
			all[e.Name] = true
		}
	}
	c := NewCatalog(all)
	b, err := json.Marshal(Message{Type: OpCatalog, Reply: "id", Catalog: &c})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("catalog message is %d bytes", len(b))
	if len(b) >= maxLine {
		t.Errorf("catalog message is %d bytes, over the %d byte line limit", len(b), maxLine)
	}
	var back Message
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(c, *back.Catalog); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestCustomPresets(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mov")
	if err := os.WriteFile(clip, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Join(dir, "presets")
	in, out, done := testSessionWith(t, preset.WithDir(storeDir))
	body := `{"id":"mine","name":"Mine","container":"mkv","video":{"codec":"hevc","rateControl":"quality",` +
		`"quality":1},"audio":{"codec":"opus","bitrate":128000}}`
	testSend(t, in,
		`{"id":"s0","op":"preset.save","presetBody":`+body+`}`,
		`{"id":"s1","op":"preset.save","presetBody":{"id":"apple","name":"Mine","container":"mp4",`+
			`"video":{"codec":"h264"}}}`,
		`{"id":"s2","op":"preset.save"}`,
		`{"id":"a0","op":"add","input":"`+clip+`","preset":"mine"}`,
		`{"id":"a1","op":"add","input":"`+clip+`","preset":"web"}`,
		`{"id":"p0","op":"presets"}`,
	)
	tests := []struct {
		Reply     string
		WantType  string
		WantError string
	}{
		{Reply: "s0", WantType: OpPresetSave},                                             // Test 0: Saved.
		{Reply: "s1", WantType: TypeError, WantError: `built-in preset: "apple"`},         // Test 1: Built-in ID.
		{Reply: "s2", WantType: TypeError, WantError: "bad request: presetBody required"}, // Test 2: No body.
		{Reply: "a0", WantType: OpAdd},                                                    // Test 3: Add by custom ID.
		{Reply: "a1", WantType: OpAdd},                                                    // Test 4: Add by built-in ID.
		{Reply: "p0", WantType: OpPresets},                                                // Test 5: Presets.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			got := waitFor(t, out, byReply(test.Reply))
			if diff := cmp.Diff([2]string{test.WantType, test.WantError}, [2]string{got.Type, got.Error}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
	saved := waitFor(t, out, byReply("s0"))
	if last := saved.Presets[len(saved.Presets)-1]; last.ID != "mine" || !last.Custom {
		t.Errorf("saved reply ends with %s custom=%v", last.ID, last.Custom)
	}
	if a := waitFor(t, out, byReply("a0")); a.Status.Job.Preset.ID != "mine" || a.Warning != "" {
		t.Errorf("custom add = %+v warning %q", a.Status.Job.Preset, a.Warning)
	}
	p := waitFor(t, out, byReply("p0"))
	if diff := cmp.Diff([]string{"web", "mine"}, p.Recent); diff != "" {
		t.Errorf("recent mismatch (-want +got):\n%s", diff)
	}
	if len(p.Presets) != len(preset.All())+1 || p.Warning != "" {
		t.Errorf("presets = %d, warning %q", len(p.Presets), p.Warning)
	}

	if err := os.WriteFile(filepath.Join(storeDir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	testSend(t, in, `{"id":"d0","op":"preset.delete","preset":"mine"}`,
		`{"id":"d1","op":"preset.delete","preset":"mine"}`)
	d := waitFor(t, out, byReply("d0"))
	if d.Type != OpPresetDelete || len(d.Presets) != len(preset.All()) || !strings.Contains(d.Warning, "broken.json") {
		t.Errorf("delete reply = %s, %d presets, warning %q", d.Type, len(d.Presets), d.Warning)
	}
	if diff := cmp.Diff([]string{"web"}, d.Recent); diff != "" {
		t.Errorf("recent after delete mismatch (-want +got):\n%s", diff)
	}
	if d1 := waitFor(t, out, byReply("d1")); d1.Error != `unknown preset: "mine"` {
		t.Errorf("second delete = %+v", d1)
	}
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
}
