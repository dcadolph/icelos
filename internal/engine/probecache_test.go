package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/probe"
)

// testCountingProber counts its calls and answers with the probed path, or err when set.
type testCountingProber struct {
	// calls is how many probes reached the prober.
	calls atomic.Int64
	// err is returned from every probe when set.
	err error
}

// Probe counts the call and answers.
func (p *testCountingProber) Probe(_ context.Context, path string) (probe.Info, error) {
	p.calls.Add(1)
	if p.err != nil {
		return probe.Info{}, p.err
	}
	return probe.Info{Format: filepath.Base(path)}, nil
}

// testWrite writes data to path or fails the test.
func testWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestProbeCache(t *testing.T) {
	t.Parallel()
	errBoom := errors.New("boom")
	tests := []struct {
		Want       error
		ProberErr  error
		Change     bool
		Missing    bool
		WantCalls  int64
		WantFormat string
	}{{ // Test 0: An unchanged file is probed once.
		WantCalls: 1, WantFormat: "clip.mov",
	}, { // Test 1: A changed file is probed again.
		Change: true, WantCalls: 2, WantFormat: "clip.mov",
	}, { // Test 2: A missing file goes to the prober every time.
		Missing: true, ProberErr: probe.ErrProbe, Want: probe.ErrProbe, WantCalls: 2,
	}, { // Test 3: A failure is not kept.
		ProberErr: errBoom, Want: errBoom, WantCalls: 2,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "clip.mov")
			if !test.Missing {
				testWrite(t, path, "one")
			}
			inner := &testCountingProber{err: test.ProberErr}
			c := NewProbeCache(inner)
			var got probe.Info
			var err error
			for i := range 2 {
				if test.Change && i == 1 {
					testWrite(t, path, "one and two")
				}
				got, err = c.Probe(t.Context(), path)
			}
			if !errors.Is(err, test.Want) {
				t.Errorf("err = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantCalls, inner.calls.Load()); diff != "" {
				t.Errorf("calls mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantFormat, got.Format); diff != "" {
				t.Errorf("format mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProbeCacheRelativePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mov")
	testWrite(t, path, "one")
	rel, err := filepath.Rel(mustGetwd(t), path)
	if err != nil {
		t.Fatalf("no relative path to the temp dir: %v", err)
	}
	inner := &testCountingProber{}
	c := NewProbeCache(inner)
	for _, p := range []string{path, rel} {
		if _, err := c.Probe(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
	if got := inner.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1: a relative and an absolute path name the same file", got)
	}
}

// mustGetwd returns the working directory or fails the test.
func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

func TestProbeCacheEvictsOldest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	paths := make([]string, probeCacheSize+1)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprintf("%03d.mov", i))
		testWrite(t, paths[i], "x")
	}
	inner := &testCountingProber{}
	c := NewProbeCache(inner)
	for _, p := range paths {
		if _, err := c.Probe(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{paths[len(paths)-1], paths[1], paths[0]} {
		if _, err := c.Probe(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := inner.calls.Load(), int64(len(paths)+1); got != want {
		t.Errorf("calls = %d, want %d: only the oldest file should be probed again", got, want)
	}
	if got := len(c.entries); got != probeCacheSize {
		t.Errorf("entries = %d, want %d", got, probeCacheSize)
	}
}

func TestProbeCacheConcurrent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "clip.mov")
	testWrite(t, path, "one")
	c := NewProbeCache(&testCountingProber{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if _, err := c.Probe(t.Context(), path); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := len(c.entries); got != 1 {
		t.Errorf("entries = %d, want 1", got)
	}
}

func TestNewProbeCachePanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("NewProbeCache(nil) did not panic")
		}
	}()
	NewProbeCache(nil)
}
