package engine

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/probe"
)

// thumbVideo returns the probe of a clip with motion video lasting dur.
func thumbVideo(dur time.Duration) probe.Info {
	return probe.Info{Duration: dur, Streams: []probe.Stream{
		{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080}}}
}

func TestThumbnail(t *testing.T) {
	t.Parallel()
	errProbe := errors.New("probe broke")
	tests := []struct {
		Want     error
		Mode     string
		Info     probe.Info
		ProbeErr error
		Missing  bool
	}{{ // Test 0: A frame is written.
		Info: thumbVideo(time.Minute),
	}, { // Test 1: A missing source.
		Info: thumbVideo(time.Minute), Missing: true, Want: ErrThumbnail,
	}, { // Test 2: A probe failure.
		ProbeErr: errProbe, Want: errProbe,
	}, { // Test 3: Audio alone has no frame.
		Info: probe.Info{Duration: time.Minute, Streams: []probe.Stream{{Kind: "audio"}}}, Want: ErrNoVideo,
	}, { // Test 4: Cover art is not motion video.
		Info: probe.Info{Duration: time.Minute, Streams: []probe.Stream{
			{Kind: "video", Disposition: probe.Disposition{AttachedPic: 1}}}}, Want: ErrNoVideo,
	}, { // Test 5: No duration leaves no place to take the frame.
		Info: thumbVideo(0), Want: ErrNoDuration,
	}, { // Test 6: An ffmpeg failure.
		Mode: "fail", Info: thumbVideo(time.Minute), Want: ErrThumbnail,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			src := filepath.Join(t.TempDir(), "in.mov")
			if !test.Missing {
				if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(t.TempDir(), "thumbs")
			f := newTestFFmpeg(t, test.Mode, test.Info, test.ProbeErr)
			got, err := f.Thumbnail(t.Context(), dir, ThumbnailRequest{Path: src})
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if err != nil {
				return
			}
			if filepath.Dir(got) != dir || filepath.Ext(got) != ".png" {
				t.Errorf("path = %s, want a PNG in %s", got, dir)
			}
			if _, err := os.Stat(got); err != nil {
				t.Errorf("thumbnail missing: %v", err)
			}
			left, err := filepath.Glob(filepath.Join(dir, ".*partial*"))
			if err != nil || len(left) != 0 {
				t.Errorf("partial files left: %v %v", left, err)
			}
		})
	}
}

func TestThumbnailCache(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "in.mov")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f := newTestFFmpeg(t, "", thumbVideo(time.Minute), nil)
	first, err := f.Thumbnail(t.Context(), dir, ThumbnailRequest{Path: src})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.bin); err != nil {
		t.Fatal(err)
	}
	second, err := f.Thumbnail(t.Context(), dir, ThumbnailRequest{Path: src})
	if err != nil {
		t.Fatalf("cached call ran ffmpeg again: %v", err)
	}
	if diff := cmp.Diff(first, second); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if _, err := f.Thumbnail(t.Context(), dir, ThumbnailRequest{Path: src, Width: 96}); err == nil {
		t.Error("another box size reused the cached image")
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(src, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Thumbnail(t.Context(), dir, ThumbnailRequest{Path: src}); err == nil {
		t.Error("a changed source reused the cached image")
	}
}

func TestThumbnailPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	_, _ = newTestFFmpeg(t, "", probe.Info{}, nil).Thumbnail(context.Background(), "", ThumbnailRequest{})
}

// TestThumbnailReal makes thumbnails of landscape and portrait test patterns with the installed
// ffmpeg and checks that each fits the default box at the source's shape.
func TestThumbnailReal(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("short mode")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	tests := []struct {
		Size       string
		WantWidth  int
		WantHeight int
	}{
		{Size: "1280x720", WantWidth: 192, WantHeight: 108}, // Test 0: Landscape fills the box.
		{Size: "720x1280", WantWidth: 61, WantHeight: 108},  // Test 1: Portrait fits its height.
		{Size: "128x72", WantWidth: 128, WantHeight: 72},    // Test 2: A small frame is not enlarged.
	}
	for _, test := range tests {
		t.Run(test.Size, func(t *testing.T) {
			t.Parallel()
			src := filepath.Join(t.TempDir(), "src.mp4")
			gen := exec.CommandContext(t.Context(), "ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "testsrc2=size="+test.Size+":rate=30:duration=1",
				"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", src)
			if out, err := gen.CombinedOutput(); err != nil {
				t.Fatalf("make source: %v: %s", err, out)
			}
			got, err := NewFFmpeg().Thumbnail(t.Context(), t.TempDir(), ThumbnailRequest{Path: src})
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(got) //nolint:gosec // Test output path.
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			cfg, err := png.DecodeConfig(file)
			if err != nil {
				t.Fatalf("decode %s: %v", got, err)
			}
			want := [2]int{test.WantWidth, test.WantHeight}
			if diff := cmp.Diff(want, [2]int{cfg.Width, cfg.Height}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if !strings.HasSuffix(got, ".png") {
				t.Errorf("path = %s", got)
			}
		})
	}
}
