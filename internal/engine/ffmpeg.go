package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dcadolph/icelos/internal/probe"
	"github.com/dcadolph/icelos/internal/progress"
)

// stderrTail is how many bytes of ffmpeg error output are kept for error messages.
const stderrTail = 4096

// FFmpeg is the Encoder that drives an ffmpeg binary.
type FFmpeg struct {
	// bin is the ffmpeg executable path.
	bin string
	// prober reads source metadata.
	prober Prober
	// mu guards avail and filters.
	mu sync.Mutex
	// avail caches the encoder list after the first successful read.
	avail EncoderSet
	// filters caches the filter list after the first successful read.
	filters FilterSet
}

// NewFFmpeg returns an FFmpeg that uses ffmpeg and ffprobe from PATH.
func NewFFmpeg() *FFmpeg {
	return WithFFmpeg("ffmpeg", ProberFunc(probe.File))
}

// WithFFmpeg returns an FFmpeg that runs bin and reads metadata through prober.
func WithFFmpeg(bin string, prober Prober) *FFmpeg {
	if bin == "" {
		panic("engine.WithFFmpeg: ffmpeg path required")
	}
	if prober == nil {
		panic("engine.WithFFmpeg: Prober required")
	}
	return &FFmpeg{bin: bin, prober: prober}
}

// Bin returns the ffmpeg executable path.
func (f *FFmpeg) Bin() string {
	return f.bin
}

// Encoders returns the encoder names the ffmpeg binary offers.
func (f *FFmpeg) Encoders(ctx context.Context) (EncoderSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.avail != nil {
		return f.avail, nil
	}
	out, err := exec.CommandContext(ctx, f.bin, "-hide_banner", "-encoders").Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrEncoders, f.bin, err)
	}
	set := ParseEncoders(out)
	if len(set) == 0 {
		return nil, fmt.Errorf("%w: %s: empty list", ErrEncoders, f.bin)
	}
	f.avail = set
	return set, nil
}

// Filters returns the filter names the ffmpeg binary offers.
func (f *FFmpeg) Filters(ctx context.Context) (FilterSet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.filters != nil {
		return f.filters, nil
	}
	out, err := exec.CommandContext(ctx, f.bin, "-hide_banner", "-filters").Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s: filter list: %w", ErrMissingFilter, f.bin, err)
	}
	f.filters = ParseFilters(out)
	return f.filters, nil
}

// checkFilters returns an error naming the first filter in names the ffmpeg binary lacks.
func (f *FFmpeg) checkFilters(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	set, err := f.Filters(ctx)
	if err != nil {
		return err
	}
	for _, name := range names {
		if !set.Has(name) {
			return fmt.Errorf("%w: %s: this ffmpeg build lacks it", ErrMissingFilter, name)
		}
	}
	return nil
}

// Encode converts job.Input to job.Output. The output appears only when the encode succeeds,
// so a failed or canceled job never leaves a partial file under the final name. A hardware
// encode that fails is retried once in software.
func (f *FFmpeg) Encode(ctx context.Context, job Job, report ReportFunc) error {
	if report == nil {
		report = func(Progress) {}
	}
	if err := job.Preset.Validate(); err != nil {
		return err
	}
	in, err := filepath.Abs(job.Input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrOutput, err)
	}
	out, err := filepath.Abs(job.Output)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrOutput, err)
	}
	if in == out {
		return fmt.Errorf("%w: %s", ErrSameFile, out)
	}
	if !job.Overwrite {
		if _, err := os.Lstat(out); err == nil {
			return fmt.Errorf("%w: %s", ErrExists, out)
		}
	}
	job.Input, job.Output = in, out

	info, err := f.prober.Probe(ctx, in)
	if err != nil {
		return err
	}
	extra := make([]probe.Info, 0, len(job.Streams.ExternalAudio))
	for _, path := range job.Streams.ExternalAudio {
		ext, err := f.prober.Probe(ctx, path)
		if err != nil {
			return err
		}
		extra = append(extra, ext)
	}
	avail, err := f.Encoders(ctx)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "icelos-")
	if err != nil {
		return fmt.Errorf("%w: temp dir: %w", ErrOutput, err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	partial := partialPath(out, job.Preset.Ext())
	defer func() { _ = os.Remove(partial) }()

	plan, err := Build(job, info, avail, partial, filepath.Join(work, "pass"), extra...)
	if err != nil {
		return err
	}
	if err := f.checkFilters(ctx, plan.Filters); err != nil {
		return err
	}
	span := job.Range.Span(info.Duration)
	err = f.run(ctx, plan, 0, span, report)
	if err != nil && plan.Hardware && ctx.Err() == nil {
		job.Hardware = false
		if plan, err = Build(job, info, avail, partial, filepath.Join(work, "pass"), extra...); err != nil {
			return err
		}
		err = f.run(ctx, plan, 0, span, report)
	}
	if err != nil {
		return err
	}
	if err := f.fit(ctx, plan, partial, span, report); err != nil {
		return err
	}
	if err := os.Rename(partial, out); err != nil {
		return fmt.Errorf("%w: %w", ErrOutput, err)
	}
	return nil
}

// fit checks a target-size output at partial against its limit. On an overshoot it reruns the
// last pass once at the bitrate plan.Fit scales from the measured size, and returns
// ErrTargetMissed if the output still lands over. Plans without a Fit return nil.
func (f *FFmpeg) fit(ctx context.Context, plan Plan, partial string, dur time.Duration, report ReportFunc) error {
	if plan.Fit == nil {
		return nil
	}
	size, err := fileSize(partial)
	if err != nil || size <= plan.Fit.Target {
		return err
	}
	bitrate, err := plan.Fit.Retry(size)
	if err != nil {
		return err
	}
	retry := plan
	retry.Passes = append(slices.Clone(plan.Passes), plan.Fit.Pass(bitrate))
	if err := f.run(ctx, retry, len(plan.Passes), dur, report); err != nil {
		return err
	}
	if size, err = fileSize(partial); err != nil {
		return err
	}
	if size > plan.Fit.Target {
		return fmt.Errorf("%w: %d bytes over the %d byte limit", ErrTargetMissed, size,
			plan.Fit.Target)
	}
	return nil
}

// fileSize returns the size of the file at path in bytes.
func fileSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrOutput, err)
	}
	return st.Size(), nil
}

// run executes the passes of plan in order from index from, reporting overall progress against
// dur.
func (f *FFmpeg) run(ctx context.Context, plan Plan, from int, dur time.Duration, report ReportFunc) error {
	passes := len(plan.Passes)
	for i, args := range plan.Passes {
		if i < from {
			continue
		}
		base := Progress{Percent: float64(i) / float64(passes), Pass: i + 1, Passes: passes,
			Encoder: plan.Encoder, Hardware: plan.Hardware}
		report(base)
		err := f.pass(ctx, args, func(u progress.Update) {
			p := base
			p.Elapsed, p.Speed, p.FPS = u.Elapsed, u.Speed, u.FPS
			done := 1.0
			if dur > 0 && !u.Done {
				done = min(1, float64(u.Elapsed)/float64(dur))
			}
			if dur > 0 || u.Done {
				p.Percent = (float64(i) + done) / float64(passes)
			}
			if dur > 0 && u.Speed > 0 {
				left := float64(dur)*float64(passes-i) - float64(u.Elapsed)
				p.ETA = time.Duration(max(0, left) / u.Speed).Round(time.Second)
			}
			report(p)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// pass runs ffmpeg once with args and feeds its progress stream to fn.
func (f *FFmpeg) pass(ctx context.Context, args []string, fn func(progress.Update)) error {
	cmd := exec.CommandContext(ctx, f.bin, args...)
	cmd.WaitDelay = 5 * time.Second
	tail := &tailBuffer{max: stderrTail}
	cmd.Stderr = tail
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrEncode, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrEncode, f.bin, err)
	}
	readErr := progress.Read(stdout, fn)
	if readErr != nil {
		_, _ = io.Copy(io.Discard, stdout) //nolint:errcheck // Draining so ffmpeg can exit.
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if msg := strings.TrimSpace(tail.String()); msg != "" {
			return fmt.Errorf("%w: %s: %w", ErrEncode, msg, err)
		}
		return fmt.Errorf("%w: %w", ErrEncode, err)
	}
	if readErr != nil {
		return fmt.Errorf("%w: progress: %w", ErrEncode, readErr)
	}
	return nil
}

// partialPath returns the hidden file next to out that ffmpeg writes before the final rename.
// It always ends in ext so ffmpeg picks the right muxer.
func partialPath(out, ext string) string {
	base := strings.TrimSuffix(filepath.Base(out), filepath.Ext(out))
	return filepath.Join(filepath.Dir(out), "."+base+".icelos-partial."+ext)
}

// tailBuffer is an io.Writer that keeps only the last max bytes written.
type tailBuffer struct {
	// max is the byte limit.
	max int
	// buf holds the kept bytes.
	buf []byte
}

// Write appends p and drops the oldest bytes past the limit.
func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(p), nil
}

// String returns the kept bytes.
func (t *tailBuffer) String() string {
	return string(t.buf)
}
