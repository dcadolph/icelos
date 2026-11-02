// Package engine turns a job into a finished output file through an Encoder.
package engine

import (
	"context"
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
	// Hardware prefers the VideoToolbox encoders and falls back to software if they fail.
	Hardware bool `json:"hardware"`
	// Overwrite replaces Output if it already exists.
	Overwrite bool `json:"overwrite"`
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
	Elapsed time.Duration `json:"elapsed"`
	// Speed is the encode speed as a multiple of real time.
	Speed float64 `json:"speed"`
	// FPS is the current encoding frame rate.
	FPS float64 `json:"fps"`
	// ETA is the estimated time left, or zero when unknown.
	ETA time.Duration `json:"eta"`
	// Encoder is the ffmpeg video encoder in use, or empty for audio or copied video.
	Encoder string `json:"encoder,omitempty"`
	// Hardware is true when Encoder runs on the media engine.
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
