package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestOutputPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want     error
		Existing []string
		Ext      string
		Dir      string
		WantName string
	}{{ // Test 0: Free name next to the input.
		Ext: "mp4", WantName: "clip.mp4",
	}, { // Test 1: Same extension as the input skips the input itself.
		Existing: []string{"clip.mov"}, Ext: "mov", WantName: "clip 2.mov",
	}, { // Test 2: Taken names count up.
		Existing: []string{"clip.mp4", "clip 2.mp4"}, Ext: "mp4", WantName: "clip 3.mp4",
	}, { // Test 3: Separate output folder.
		Ext: "m4a", Dir: "out", WantName: "out/clip.m4a",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			in := filepath.Join(root, "clip.mov")
			for _, name := range append([]string{"clip.mov"}, test.Existing...) {
				if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			dir := ""
			if test.Dir != "" {
				dir = filepath.Join(root, test.Dir)
			}
			got, err := OutputPath(in, test.Ext, dir)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(filepath.Join(root, test.WantName), got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
