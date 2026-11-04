package tooltest

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// recorder is a testing.TB stand-in that records a Fatalf call instead of ending the test.
type recorder struct {
	// TB satisfies the interface's unexported method and is never called.
	testing.TB
	// Calls counts the Fatalf calls.
	Calls int
	// Message is the formatted text of the last Fatalf call.
	Message string
}

// Helper does nothing, since the recorder has no call stack to trim.
func (r *recorder) Helper() {}

// Fatalf records the formatted message.
func (r *recorder) Fatalf(format string, args ...any) {
	r.Calls++
	r.Message = fmt.Sprintf(format, args...)
}

func TestRequire(t *testing.T) {
	t.Parallel()
	const missing = "icelos-no-such-tool"
	tests := []struct {
		WantMessage string
		Names       []string
		WantCalls   int
	}{{ // Test 0: No tools asked for.
		Names: nil, WantCalls: 0, WantMessage: "",
	}, { // Test 1: A tool that go test puts on PATH.
		Names: []string{"go"}, WantCalls: 0, WantMessage: "",
	}, { // Test 2: A missing tool after a present one.
		Names: []string{"go", missing}, WantCalls: 1,
		WantMessage: missing + " is not on PATH; the real-ffmpeg tests run or fail, never skip",
	}, { // Test 3: Only the first missing tool is named.
		Names: []string{missing, missing + "-two"}, WantCalls: 1,
		WantMessage: missing + " is not on PATH; the real-ffmpeg tests run or fail, never skip",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			r := &recorder{}
			Require(r, test.Names...)
			if diff := cmp.Diff(test.WantCalls, r.Calls); diff != "" {
				t.Errorf("Fatalf calls mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantMessage, r.Message); diff != "" {
				t.Errorf("message mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
