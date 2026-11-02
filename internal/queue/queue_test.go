package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/preset"
)

// errTestEncode is what the test encoder returns for inputs named fail.
var errTestEncode = errors.New("test encode failed") //nolint:gochecknoglobals // Not modified, simplifies testing.

// testEncoder fails inputs named fail, blocks inputs named block until canceled or released,
// and finishes everything else after one progress report.
type testEncoder struct {
	// release unblocks block jobs without error when closed.
	release chan struct{}
	// mu guards running and peak.
	mu sync.Mutex
	// running counts encodes in flight.
	running int
	// peak is the highest running count seen.
	peak int
}

// newTestEncoder returns a testEncoder with an open release channel.
func newTestEncoder() *testEncoder {
	return &testEncoder{release: make(chan struct{})}
}

// Encode implements engine.Encoder.
func (e *testEncoder) Encode(ctx context.Context, job engine.Job, report engine.ReportFunc) error {
	e.mu.Lock()
	e.running++
	e.peak = max(e.peak, e.running)
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.running--
		e.mu.Unlock()
	}()
	report(engine.Progress{Percent: 0.5, Pass: 1, Passes: 1})
	switch strings.TrimSuffix(filepath.Base(job.Input), filepath.Ext(job.Input)) {
	case "fail":
		return errTestEncode
	case "block":
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.release:
		}
	}
	return nil
}

// testFiles creates empty files with the given names in a temp dir and returns their paths.
func testFiles(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = filepath.Join(dir, n)
		if err := os.WriteFile(paths[i], nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

// testJob returns a job for input using the web preset.
func testJob(t *testing.T, input string) engine.Job {
	t.Helper()
	p, err := preset.Lookup("web")
	if err != nil {
		t.Fatal(err)
	}
	return engine.Job{Input: input, Preset: p}
}

// waitState polls q until job id reaches want or the test times out.
func waitState(t *testing.T, q *Queue, id string, want State) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, st := range q.List() {
			if st.ID == id && st.State == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %s: %+v", id, want, q.List())
}

func TestAddResolvesOutputs(t *testing.T) {
	t.Parallel()
	paths := testFiles(t, "clip.mov", "clip.mp4")
	q := New(newTestEncoder(), nil)
	var got []string
	for range 2 {
		st, err := q.Add(testJob(t, paths[0]), "")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.Base(st.Job.Output))
	}
	if diff := cmp.Diff([]string{"clip 2.mp4", "clip 3.mp4"}, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestAddErrors(t *testing.T) {
	t.Parallel()
	paths := testFiles(t, "a.mov")
	bad, err := preset.Lookup("remux")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		Want error
		Job  func(engine.Job) engine.Job
	}{{ // Test 0: Missing input.
		Job: func(j engine.Job) engine.Job { j.Input += ".nope"; return j }, Want: ErrInput,
	}, { // Test 1: Directory input.
		Job: func(j engine.Job) engine.Job { j.Input = filepath.Dir(j.Input); return j }, Want: ErrInput,
	}, { // Test 2: Invalid preset.
		Job: func(j engine.Job) engine.Job { j.Preset = bad.WithTargetSize(1); return j }, Want: preset.ErrInvalid,
	}, { // Test 3: Explicit output already claimed by the first job.
		Job:  func(j engine.Job) engine.Job { j.Output = filepath.Join(filepath.Dir(j.Input), "a.mp4"); return j },
		Want: ErrDuplicateOutput,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			q := New(newTestEncoder(), nil)
			if _, err := q.Add(testJob(t, paths[0]), ""); err != nil {
				t.Fatal(err)
			}
			if _, err := q.Add(test.Job(testJob(t, paths[0])), ""); !errors.Is(err, test.Want) {
				t.Errorf("error = %v, want %v", err, test.Want)
			}
		})
	}
}

func TestDrain(t *testing.T) {
	t.Parallel()
	paths := testFiles(t, "ok.mov", "fail.mov", "ok2.mov")
	var mu sync.Mutex
	var events []State
	q := New(newTestEncoder(), func(st Status) {
		mu.Lock()
		defer mu.Unlock()
		if st.ID == "2" {
			events = append(events, st.State)
		}
	})
	for _, p := range paths {
		if _, err := q.Add(testJob(t, p), ""); err != nil {
			t.Fatal(err)
		}
	}
	q.Drain(t.Context())
	var got []State
	for _, st := range q.List() {
		got = append(got, st.State)
	}
	if diff := cmp.Diff([]State{StateDone, StateFailed, StateDone}, got); diff != "" {
		t.Errorf("states mismatch (-want +got):\n%s", diff)
	}
	if msg := q.List()[1].Error; msg != errTestEncode.Error() {
		t.Errorf("error = %q, want %q", msg, errTestEncode.Error())
	}
	if diff := cmp.Diff([]State{StateQueued, StateRunning, StateRunning, StateFailed}, events); diff != "" {
		t.Errorf("events mismatch (-want +got):\n%s", diff)
	}
	if p := q.List()[0].Progress.Percent; p != 1 {
		t.Errorf("done percent = %v, want 1", p)
	}
}

func TestCancel(t *testing.T) {
	t.Parallel()
	paths := testFiles(t, "block.mov", "later.mov")
	enc := newTestEncoder()
	q := New(enc, nil)
	first, err := q.Add(testJob(t, paths[0]), "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := q.Add(testJob(t, paths[1]), "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		q.Drain(t.Context())
		close(done)
	}()
	waitState(t, q, first.ID, StateRunning)
	if err := q.Cancel(second.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Cancel(first.ID); err != nil {
		t.Fatal(err)
	}
	<-done
	var got []State
	for _, st := range q.List() {
		got = append(got, st.State)
	}
	if diff := cmp.Diff([]State{StateCanceled, StateCanceled}, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if err := q.Cancel(first.ID); err != nil {
		t.Errorf("cancel finished job: %v", err)
	}
	if err := q.Cancel("99"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("error = %v, want %v", err, ErrUnknownJob)
	}
	q.Clear()
	if n := len(q.List()); n != 0 {
		t.Errorf("after Clear, %d jobs remain", n)
	}
	if _, err := q.Add(engine.Job{Input: paths[0], Output: first.Job.Output, Preset: first.Job.Preset}, ""); err != nil {
		t.Errorf("finished job still holds its output: %v", err)
	}
}

func TestServeWakesAndStops(t *testing.T) {
	t.Parallel()
	paths := testFiles(t, "a.mov", "block.mov")
	q := New(newTestEncoder(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		q.Serve(ctx)
		close(done)
	}()
	st, err := q.Add(testJob(t, paths[0]), "")
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, q, st.ID, StateDone)
	blocked, err := q.Add(testJob(t, paths[1]), "")
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, q, blocked.ID, StateRunning)
	cancel()
	<-done
	waitState(t, q, blocked.ID, StateCanceled)
}

func TestWorkersRunInParallel(t *testing.T) {
	t.Parallel()
	paths := testFiles(t, "block.mov", "block.mkv", "block.avi")
	enc := newTestEncoder()
	q := WithWorkers(enc, 2, nil)
	var ids []string
	for _, p := range paths {
		st, err := q.Add(testJob(t, p), "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, st.ID)
	}
	done := make(chan struct{})
	go func() {
		q.Drain(t.Context())
		close(done)
	}()
	waitState(t, q, ids[0], StateRunning)
	waitState(t, q, ids[1], StateRunning)
	close(enc.release)
	<-done
	enc.mu.Lock()
	defer enc.mu.Unlock()
	if enc.peak != 2 {
		t.Errorf("peak parallel = %d, want 2", enc.peak)
	}
	for _, st := range q.List() {
		if st.State != StateDone {
			t.Errorf("job %s = %s, want done", st.ID, st.State)
		}
	}
}

func TestWithWorkersPanics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Enc     engine.Encoder
		Workers int
	}{{ // Test 0: Nil encoder.
		Enc: nil, Workers: 1,
	}, { // Test 1: Zero workers.
		Enc: newTestEncoder(), Workers: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Error("expected panic")
				}
			}()
			WithWorkers(test.Enc, test.Workers, nil)
		})
	}
}

func TestStateFinished(t *testing.T) {
	t.Parallel()
	for state, want := range map[State]bool{
		StateQueued: false, StateRunning: false, StateDone: true, StateFailed: true, StateCanceled: true,
	} {
		if got := state.Finished(); got != want {
			t.Errorf("%s.Finished() = %v, want %v", state, got, want)
		}
	}
}
