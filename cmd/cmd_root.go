package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dcadolph/icelos/cmd/config"
	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/jsonutil"
	"github.com/dcadolph/icelos/internal/probe"
)

// rootCmd builds the icelos root command and its subcommands.
func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "icelos",
		Short: "Convert video and audio on macOS.",
		Long: `icelos converts video and audio with ffmpeg, using the Apple media engine when it
can and software encoders when it must.

Results go to stdout as JSON and progress goes to stderr. Every flag can also be set
through an ICELOS_ environment variable, e.g. ICELOS_OUT_DIR for --out-dir.`,
		Version:       resolveVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().AddFlag(&config.FlagPretty)
	cmd.PersistentFlags().AddFlag(&config.FlagFFmpeg)
	cmd.PersistentFlags().AddFlag(&config.FlagFFprobe)
	cmd.AddCommand(convertCmd(), presetsCmd(), probeCmd(), encodersCmd(), serveCmd())
	return cmd
}

// resolveTool returns the configured path for a tool, else the copy beside the running
// binary, which is where the app bundle keeps it, else the bare name for a PATH lookup.
func resolveTool(name, configured string) string {
	if configured != "" {
		return configured
	}
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	candidate := filepath.Join(filepath.Dir(exe), name)
	if fi, err := os.Stat(candidate); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
		return candidate
	}
	return name
}

// newEngine returns the ffmpeg driver and prober for the configured tools.
func newEngine() (*engine.FFmpeg, engine.Prober) {
	ffprobe := resolveTool("ffprobe", config.FFprobe())
	prober := engine.ProberFunc(func(ctx context.Context, path string) (probe.Info, error) {
		return probe.Run(ctx, ffprobe, path)
	})
	return engine.WithFFmpeg(resolveTool("ffmpeg", config.FFmpeg()), prober), prober
}

// writeJSON writes v to w as one JSON document.
func writeJSON(w io.Writer, v any) error {
	b, err := jsonutil.Marshal(v, config.Pretty())
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
