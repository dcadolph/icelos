// Package tooltest fails a test when a command-line tool it needs is not installed, so the tests
// that drive the real ffmpeg run or fail and never skip, and writes the fake tools other tests
// run in its place.
package tooltest

import (
	"os"
	"os/exec"
	"syscall"
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

// WriteScript writes body to path as an executable script. It holds syscall.ForkLock while the
// file is open, so a process that a parallel test starts at that moment cannot inherit the write
// handle, which on Linux makes running the script fail with "text file busy".
func WriteScript(t testing.TB, path, body string) {
	t.Helper()
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil { //nolint:gosec // Test script.
		t.Fatalf("write script: %v", err)
	}
}
