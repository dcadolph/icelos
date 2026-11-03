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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/format"
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

// testSession starts a server with a fake encoder and returns its input and output.
func testSession(t *testing.T) (*io.PipeWriter, *testOutput, <-chan error) {
	t.Helper()
	enc := engine.EncoderFunc(func(ctx context.Context, job engine.Job, report engine.ReportFunc) error {
		report(engine.Progress{Percent: 0.5, Pass: 1, Passes: 1})
		if strings.Contains(job.Input, "block") {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	})
	prober := engine.ProberFunc(func(_ context.Context, path string) (probe.Info, error) {
		if strings.HasSuffix(path, "missing.mov") {
			return probe.Info{}, probe.ErrProbe
		}
		return probe.Info{Format: "mov", Duration: time.Second}, nil
	})
	encoders := func(context.Context) (engine.EncoderSet, error) {
		return engine.EncoderSet{"libx264": true, "hevc_videotoolbox": true, "png": true}, nil
	}
	out := &testOutput{}
	srv := New(out, Config{Encoder: enc, Prober: prober, Encoders: encoders, Version: "v0.0.0-test"})
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
	if c := waitFor(t, out, byReply("c")); !cmp.Equal(c.Encoders, []string{"hevc_videotoolbox", "libx264"}) {
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
	srv := New(testFailWriter{}, Config{Encoder: enc, Prober: prober,
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
	}{{ // Test 0: libx265 makes HEVC available, and theora has no encoder icelos drives.
		In:            engine.EncoderSet{"libx265": true, "libtheora": true, "aac": true},
		WantAvailable: map[string]bool{"hevc": true, "theora": false, "aac": true, "h264": false},
	}, { // Test 1: Without libx265 HEVC is unavailable.
		In:            engine.EncoderSet{"libtheora": true, "aac": true},
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
