package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/dcadolph/icelos/cmd/config"
	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/format"
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
			streams, err := selectedStreams()
			if err != nil {
				return err
			}
			out := config.Output()
			if out != "" && len(args) > 1 {
				return fmt.Errorf("%w: --output takes a single input, got %d", errUsage, len(args))
			}
			enc, prober := newEngine()
			prog := newProgressPrinter(cmd.ErrOrStderr())
			q := queue.WithWorkers(enc, prober, max(1, config.Workers()), prog.Print)
			for _, in := range args {
				job := engine.Job{Input: in, Output: out, Preset: p, Hardware: config.Hardware(),
					Overwrite: config.Overwrite(), Streams: streams,
					SplitChapters: config.SplitChapters(), Passthrough: config.Passthrough()}
				if _, err := q.Add(cmd.Context(), job, config.OutDir()); err != nil {
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
		config.KeyHardware, config.KeyOverwrite, config.KeyWorkers, config.KeyContainer,
		config.KeyVideoCodec, config.KeyAudioCodec, config.KeyResolution, config.KeyFPS,
		config.KeyVideoBitrate, config.KeyAudioBitrate, config.KeyAudioTracks, config.KeySubtitles,
		config.KeyThreads, config.KeySplitChapters, config.KeyPassthrough} {
		cmd.Flags().AddFlag(config.Lookup(f))
	}
	return cmd
}

// selectedPreset returns the configured preset with any target size and format flags applied.
func selectedPreset() (preset.Preset, error) {
	store, err := preset.NewStore()
	if err != nil {
		return preset.Preset{}, err
	}
	p, err := store.Lookup(config.Preset())
	if err != nil {
		return preset.Preset{}, err
	}
	if p, err = applyFormatFlags(p); err != nil {
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

// sizePattern matches an exact WxH resolution.
var sizePattern = regexp.MustCompile(`^(\d+)[xX](\d+)$`) //nolint:gochecknoglobals // Read-only pattern.

// applyFormatFlags applies the container, codec, resolution, frame rate, and bitrate flags to p.
// Unset flags change nothing.
func applyFormatFlags(p preset.Preset) (preset.Preset, error) {
	if id := config.Container(); id != "" {
		p.Container = id
		if box, err := format.ContainerByID(id); err == nil {
			p.Video.Codec = fitCodec(p.Video.Codec, box.Video, p.Video.Codec != "")
			p.Audio.Codec = fitCodec(p.Audio.Codec, box.Audio, p.Audio.Codec != "")
		}
	}
	if id := config.VideoCodec(); id != "" {
		p.Video.Codec = id
	}
	if id := config.AudioCodec(); id != "" {
		p.Audio.Codec = id
		if id == "none" {
			p.Audio.Codec = ""
		}
	}
	if r := config.Resolution(); r != "" {
		if err := applyResolution(&p.Video, r); err != nil {
			return preset.Preset{}, err
		}
	}
	if s := config.FPS(); s != "" {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f < 0 {
			return preset.Preset{}, fmt.Errorf("%w: --fps %q", errUsage, s)
		}
		p.Video.FPS = f
	}
	if s := config.VideoBitrate(); s != "" {
		n, err := parseBitrate(s)
		if err != nil {
			return preset.Preset{}, fmt.Errorf("%w: --video-bitrate %q", errUsage, s)
		}
		p.Video.Bitrate = n
		if p.Video.RateControl == "" || p.Video.RateControl == format.RateQuality {
			p.Video.RateControl = format.RateVBR
		}
	}
	if s := config.AudioBitrate(); s != "" {
		n, err := parseBitrate(s)
		if err != nil {
			return preset.Preset{}, fmt.Errorf("%w: --audio-bitrate %q", errUsage, s)
		}
		p.Audio.Bitrate = n
	}
	return p, nil
}

// fitCodec returns codec when allowed lists it or when it is empty or copy, else the first
// allowed codec, or empty when the container allows none.
func fitCodec(codec string, allowed []string, set bool) string {
	if !set || codec == format.Copy || slices.Contains(allowed, codec) {
		return codec
	}
	if len(allowed) == 0 {
		return ""
	}
	return allowed[0]
}

// applyResolution sets the size fields of v from a resolution ID or an exact WxH.
func applyResolution(v *format.Video, r string) error {
	if m := sizePattern.FindStringSubmatch(r); m != nil {
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		v.Width, v.Height, v.MaxWidth, v.MaxHeight = w, h, 0, 0
		return nil
	}
	for _, res := range format.Options().Resolutions {
		if res.ID != r {
			continue
		}
		v.Width, v.Height = 0, 0
		v.MaxWidth, v.MaxHeight = res.Width, res.Height
		return nil
	}
	return fmt.Errorf("%w: --resolution %q", errUsage, r)
}

// parseBitrate reads a bits-per-second number with an optional k or M suffix.
func parseBitrate(s string) (int, error) {
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "k"), strings.HasSuffix(s, "K"):
		mult, s = 1e3, s[:len(s)-1]
	case strings.HasSuffix(s, "M"), strings.HasSuffix(s, "m"):
		mult, s = 1e6, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, errUsage
	}
	return int(f * mult), nil
}

// selectedStreams returns the stream selection built from the audio, subtitle, and thread flags.
func selectedStreams() (engine.Streams, error) {
	var st engine.Streams
	st.Threads = config.Threads()
	if st.Threads < 0 {
		return engine.Streams{}, fmt.Errorf("%w: --threads %d", errUsage, st.Threads)
	}
	if s := config.AudioTracks(); s != "" {
		if strings.EqualFold(s, "all") {
			st.AudioTracks = []int{engine.AllTracks}
		} else {
			for _, f := range strings.Split(s, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(f))
				if err != nil || n < 1 {
					return engine.Streams{}, fmt.Errorf("%w: --audio-tracks %q", errUsage, s)
				}
				st.AudioTracks = append(st.AudioTracks, n-1)
			}
		}
	}
	switch s := config.Subtitles(); strings.ToLower(s) {
	case "":
	case "none":
		st.Subtitles.Mode = engine.SubtitlesNone
	case "keep":
		st.Subtitles.Mode = engine.SubtitlesKeep
	case "burn":
		st.Subtitles.Mode = engine.SubtitlesBurn
	default:
		st.Subtitles = engine.Subtitles{Mode: engine.SubtitlesKeep, External: s}
	}
	return st, nil
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
