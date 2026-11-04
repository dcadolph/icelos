// Package engine turns a job into a finished output file through an Encoder.
package engine

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// Job is one conversion request.
type Job struct {
	// Input is the source file path.
	Input string `json:"input"`
	// Output is the destination file path.
	Output string `json:"output"`
	// Preset is the conversion target.
	Preset preset.Preset `json:"preset"`
	// Hardware prefers the hardware encoders and falls back to software if they fail.
	Hardware bool `json:"hardware"`
	// Passthrough copies every stream that already matches the preset instead of encoding it.
	Passthrough bool `json:"passthrough,omitempty"`
	// Overwrite replaces Output if it already exists.
	Overwrite bool `json:"overwrite"`
	// Streams chooses the tracks, subtitles, chapters, and metadata the output carries.
	Streams Streams `json:"streams,omitzero"`
	// Range limits the output to part of the input, zero for the whole input.
	Range Range `json:"range,omitzero"`
	// SplitChapters asks the queue to add one job per chapter of the input instead of this one.
	// The engine never reads it.
	SplitChapters bool `json:"splitChapters,omitempty"`
}

// Range is a span of the input's timeline.
type Range struct {
	// Start is where the output begins, zero for the start of the input.
	Start time.Duration `json:"startNs,omitempty"`
	// End is where the output ends, zero for the end of the input.
	End time.Duration `json:"endNs,omitempty"`
}

// Validate reports whether r is a forward span.
func (r Range) Validate() error {
	switch {
	case r.Start < 0:
		return fmt.Errorf("%w: start %s before the input", ErrRange, r.Start)
	case r.End != 0 && r.End <= r.Start:
		return fmt.Errorf("%w: end %s not after start %s", ErrRange, r.End, r.Start)
	}
	return nil
}

// Span returns how long the output plays when the whole input plays for total, or zero when
// that is unknown.
func (r Range) Span(total time.Duration) time.Duration {
	end := r.End
	if end == 0 || (total > 0 && end > total) {
		end = total
	}
	return max(0, end-r.Start)
}

// args returns the input options that read only r from the next input: -ss for the start and
// -to for the end, both on the input's own timeline.
func (r Range) args() []string {
	var args []string
	if r.Start > 0 {
		args = append(args, "-ss", seconds(r.Start))
	}
	if r.End > 0 {
		args = append(args, "-to", seconds(r.End))
	}
	return args
}

// seconds formats d as decimal seconds for ffmpeg time options.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
}

// Progress is one status report during an encode.
type Progress struct {
	// Percent is the overall completion from 0 to 1 across every pass.
	Percent float64 `json:"percent"`
	// Pass is the current pass, starting at 1.
	Pass int `json:"pass"`
	// Passes is the total pass count.
	Passes int `json:"passes"`
	// Elapsed is the media time encoded so far in this pass.
	Elapsed time.Duration `json:"elapsedNs"`
	// Speed is the encode speed as a multiple of real time.
	Speed float64 `json:"speed"`
	// FPS is the current encoding frame rate.
	FPS float64 `json:"fps"`
	// ETA is the estimated time left, or zero when unknown.
	ETA time.Duration `json:"etaNs"`
	// Encoder is the ffmpeg video encoder in use, or empty for audio or copied video.
	Encoder string `json:"encoder,omitempty"`
	// Hardware is true when Encoder runs on a media engine or GPU.
	Hardware bool `json:"hardware"`
}

// ReportFunc receives progress reports. It must return quickly.
type ReportFunc func(Progress)

// Encoder converts one job to its output file.
type Encoder interface {
	// Encode runs job to completion, calling report as work advances. A nil report is allowed.
	Encode(ctx context.Context, job Job, report ReportFunc) error
}

// EncoderFunc adapts a function to the Encoder interface.
type EncoderFunc func(ctx context.Context, job Job, report ReportFunc) error

// Encode calls f.
func (f EncoderFunc) Encode(ctx context.Context, job Job, report ReportFunc) error {
	return f(ctx, job, report)
}

// Prober reads media metadata for a path.
type Prober interface {
	// Probe returns the metadata for path.
	Probe(ctx context.Context, path string) (probe.Info, error)
}

// ProberFunc adapts a function to the Prober interface.
type ProberFunc func(ctx context.Context, path string) (probe.Info, error)

// Probe calls f.
func (f ProberFunc) Probe(ctx context.Context, path string) (probe.Info, error) {
	return f(ctx, path)
}
