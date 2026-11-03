package engine

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// update rewrites the golden files instead of comparing against them.
var update = flag.Bool("update", false, "Rewrite golden files.") //nolint:gochecknoglobals // Test flag.

// goldenInfo returns the fixed 90 second 1080p clip with 6-channel AAC audio.
func goldenInfo() probe.Info {
	return probe.Info{Duration: 90 * time.Second, Streams: []probe.Stream{
		{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
		{Index: 1, Kind: "audio", Codec: "aac", Channels: 6},
	}}
}

// TestGolden compares the planned ffmpeg arguments for each preset to the golden files.
func TestGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		PresetID string
		File     string
		Hardware bool
	}{
		{PresetID: "apple", File: "apple.txt"},                    // Test 0: Apple preset in software.
		{PresetID: "compatible", File: "compatible.txt"},          // Test 1: Compatible preset.
		{PresetID: "web", File: "web.txt"},                        // Test 2: Web preset.
		{PresetID: "tiny", File: "tiny.txt"},                      // Test 3: Tiny preset in software.
		{PresetID: "target-size", File: "target-size.txt"},        // Test 4: Target size preset.
		{PresetID: "remux", File: "remux.txt"},                    // Test 5: Remux preset.
		{PresetID: "audio", File: "audio.txt"},                    // Test 6: Audio preset.
		{PresetID: "mp3", File: "mp3.txt"},                        // Test 7: MP3 preset.
		{PresetID: "apple", File: "apple-hw.txt", Hardware: true}, // Test 8: Apple with hardware.
		{PresetID: "tiny", File: "tiny-hw.txt", Hardware: true},   // Test 9: Tiny with hardware.
	}
	for _, test := range tests {
		t.Run(test.File, func(t *testing.T) {
			t.Parallel()
			p, err := preset.Lookup(test.PresetID)
			if err != nil {
				t.Fatalf("lookup %s: %v", test.PresetID, err)
			}
			if p.ID == preset.TargetSizeID {
				p = p.WithTargetSize(50_000_000)
			}
			job := Job{Input: "in.mov", Output: "out." + p.Ext(), Preset: p, Hardware: test.Hardware}
			plan, err := Build(job, goldenInfo(), testAvail(), "out."+p.Ext(), "passlog")
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			lines := make([]string, 0, len(plan.Passes))
			for _, args := range plan.Passes {
				lines = append(lines, strings.Join(args, " "))
			}
			got := strings.Join(lines, "\n") + "\n"
			path := filepath.Join("testdata", "golden", test.File)
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if diff := cmp.Diff(string(want), got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
