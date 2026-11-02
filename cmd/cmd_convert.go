package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/dcadolph/icelos/cmd/config"
	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/queue"
)

// convertCmd builds the convert subcommand.
func convertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "convert FILE...",
		Short: "Convert one or more files with a preset.",
		Long: `convert runs each file through the chosen preset and writes the result next to
the input, or into --out-dir. Existing files are never replaced unless --overwrite is set,
and a taken name gets a number, e.g. "clip 2.mp4". A failed or interrupted encode leaves
nothing behind.`,
		Example: `  icelos convert clip.mov
  icelos convert --preset tiny *.mov
  icelos convert --preset target-size --size 25MB talk.mkv`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := selectedPreset()
			if err != nil {
				return err
			}
			out := config.Output()
			if out != "" && len(args) > 1 {
				return fmt.Errorf("%w: --output takes a single input, got %d", errUsage, len(args))
			}
			enc, _ := newEngine()
			prog := newProgressPrinter(cmd.ErrOrStderr())
			q := queue.WithWorkers(enc, max(1, config.Workers()), prog.Print)
			for _, in := range args {
				job := engine.Job{Input: in, Output: out, Preset: p, Hardware: config.Hardware(),
					Overwrite: config.Overwrite()}
				if _, err := q.Add(job, config.OutDir()); err != nil {
					return fmt.Errorf("%s: %w", in, err)
				}
			}
			q.Drain(cmd.Context())
			results := q.List()
			if err := writeJSON(cmd.OutOrStdout(), results); err != nil {
				return err
			}
			if cmd.Context().Err() != nil {
				return errInterrupted
			}
			for _, st := range results {
				if st.State == queue.StateFailed {
					return errJobsFailed
				}
			}
			return nil
		},
	}
	for _, f := range []string{config.KeyPreset, config.KeySize, config.KeyOutDir, config.KeyOutput,
		config.KeyHardware, config.KeyOverwrite, config.KeyWorkers} {
		cmd.Flags().AddFlag(config.Lookup(f))
	}
	return cmd
}

// selectedPreset returns the configured preset with any target size applied.
func selectedPreset() (preset.Preset, error) {
	p, err := preset.Lookup(config.Preset())
	if err != nil {
		return preset.Preset{}, err
	}
	if s := config.Size(); s != "" {
		n, err := preset.ParseSize(s)
		if err != nil {
			return preset.Preset{}, err
		}
		p = p.WithTargetSize(n)
	}
	if p.ID == preset.TargetSizeID && p.TargetBytes == 0 {
		return preset.Preset{}, fmt.Errorf("%w: %s needs --size", errUsage, p.ID)
	}
	if err := p.Validate(); err != nil {
		if errors.Is(err, preset.ErrInvalid) && p.TargetBytes > 0 {
			return preset.Preset{}, fmt.Errorf("%w: --size needs a video preset that encodes", errUsage)
		}
		return preset.Preset{}, err
	}
	return p, nil
}

// progressPrinter writes human progress lines to stderr.
type progressPrinter struct {
	// mu serializes writes.
	mu sync.Mutex
	// w is the destination.
	w io.Writer
	// live is true when w is a terminal, so the line is redrawn in place.
	live bool
	// last is when each job last printed a progress line.
	last map[string]time.Time
}

// newProgressPrinter returns a printer for w.
func newProgressPrinter(w io.Writer) *progressPrinter {
	live := false
	if f, ok := w.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			live = true
		}
	}
	return &progressPrinter{w: w, live: live, last: map[string]time.Time{}}
}

// Print reports one status change. Progress lines are limited to one per second per job
// when w is not a terminal.
func (p *progressPrinter) Print(st queue.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()
	name := filepath.Base(st.Job.Input)
	switch st.State {
	case queue.StateQueued:
		return
	case queue.StateRunning:
		if !p.live && time.Since(p.last[st.ID]) < time.Second {
			return
		}
		p.last[st.ID] = time.Now()
		line := fmt.Sprintf("%s: %3.0f%%", name, st.Progress.Percent*100)
		if st.Progress.Speed > 0 {
			line += fmt.Sprintf("  %.1fx", st.Progress.Speed)
		}
		if st.Progress.ETA > 0 {
			line += "  " + st.Progress.ETA.String() + " left"
		}
		if st.Progress.Encoder != "" {
			line += "  " + st.Progress.Encoder
		}
		if p.live {
			_, _ = fmt.Fprintf(p.w, "\r\033[K%s", line)
			return
		}
		_, _ = fmt.Fprintln(p.w, line)
	default:
		prefix := ""
		if p.live {
			prefix = "\r\033[K"
		}
		msg := fmt.Sprintf("%s%s: %s", prefix, name, st.State)
		switch st.State {
		case queue.StateDone:
			msg += " -> " + st.Job.Output
		case queue.StateFailed:
			msg += ": " + st.Error
		}
		_, _ = fmt.Fprintln(p.w, msg)
	}
}
