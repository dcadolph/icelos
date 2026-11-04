package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dcadolph/icelos/internal/util/proc"
)

// Thumbnail defaults, the same as the Mac app's: the row tile at twice its 96 by 54 point
// size, taken two seconds in or halfway through a shorter file.
const (
	// ThumbnailWidth is the default box width in pixels.
	ThumbnailWidth = 192
	// ThumbnailHeight is the default box height in pixels.
	ThumbnailHeight = 108
	// thumbnailAt is how far into the file the default frame is taken.
	thumbnailAt = 2 * time.Second
)

// ThumbnailRequest says which frame of a file to make into an image.
type ThumbnailRequest struct {
	// Path is the source file.
	Path string
	// Width is the box width in pixels the image fits inside. Zero is ThumbnailWidth.
	Width int
	// Height is the box height in pixels the image fits inside. Zero is ThumbnailHeight.
	Height int
	// At is where in the file the frame is taken. Zero is two seconds in, or halfway through a
	// shorter file.
	At time.Duration
}

// Thumbnail writes one frame of r.Path as a PNG under dir and returns its path. The image fits
// inside the box without being enlarged. A file is reused while the source path, size, and
// modification time and the request stay the same.
func (f *FFmpeg) Thumbnail(ctx context.Context, dir string, r ThumbnailRequest) (string, error) {
	if dir == "" {
		panic("engine.FFmpeg.Thumbnail: cache dir required")
	}
	src, err := filepath.Abs(r.Path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrThumbnail, err)
	}
	st, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrThumbnail, err)
	}
	if r.Width <= 0 {
		r.Width = ThumbnailWidth
	}
	if r.Height <= 0 {
		r.Height = ThumbnailHeight
	}
	if r.At <= 0 {
		info, err := f.prober.Probe(ctx, src)
		if err != nil {
			return "", err
		}
		if _, ok := info.FirstVideo(); !ok {
			return "", fmt.Errorf("%w: %s", ErrNoVideo, src)
		}
		if info.Duration <= 0 {
			return "", fmt.Errorf("%w: %s", ErrNoDuration, src)
		}
		r.At = min(thumbnailAt, info.Duration/2)
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d\x00%d\x00%dx%d\x00%d", src, st.Size(),
		st.ModTime().UnixNano(), r.Width, r.Height, r.At))
	name := hex.EncodeToString(sum[:])
	out := filepath.Join(dir, name+".png")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // A cache folder others may read.
		return "", fmt.Errorf("%w: %w", ErrThumbnail, err)
	}
	partial := filepath.Join(dir, "."+name+".partial.png")
	defer func() { _ = os.Remove(partial) }()
	box := "scale=w='min(iw," + strconv.Itoa(r.Width) + ")':h='min(ih," + strconv.Itoa(r.Height) +
		")':force_original_aspect_ratio=decrease"
	cmd := proc.Command(ctx, f.bin, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
		"-ss", seconds(r.At), "-i", src, "-frames:v", "1", "-vf", box, "-f", "image2", partial)
	tail := &tailBuffer{max: stderrTail}
	cmd.Stderr = tail
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%w: %s: %w", ErrThumbnail, strings.TrimSpace(tail.String()), err)
	}
	if err := os.Rename(partial, out); err != nil {
		return "", fmt.Errorf("%w: %w", ErrThumbnail, err)
	}
	return out, nil
}
