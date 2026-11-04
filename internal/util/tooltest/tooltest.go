// Package tooltest fails a test when a command-line tool it needs is not installed, so the tests
// that drive the real ffmpeg run or fail and never skip.
package tooltest

import (
	"os/exec"
	"testing"
)

// Require fails the test through t.Fatalf, naming the first tool in names that is not on PATH.
func Require(t testing.TB, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("%s is not on PATH; the real-ffmpeg tests run or fail, never skip", name)
			return
		}
	}
}
