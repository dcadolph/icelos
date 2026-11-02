package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// toolDirs are checked after PATH, since apps opened from Finder get a minimal PATH.
var toolDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"} //nolint:gochecknoglobals // Read-only.

// resolveTool returns the configured path for a tool, else the copy beside the running
// binary, which is where the app bundle keeps it, else a PATH match, else a Homebrew copy,
// else the bare name so the error names the missing tool.
func resolveTool(name, configured string) string {
	if configured != "" {
		return configured
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		if candidate := filepath.Join(filepath.Dir(exe), name); isExecutable(candidate) {
			return candidate
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range toolDirs {
		if candidate := filepath.Join(dir, name); isExecutable(candidate) {
			return candidate
		}
	}
	return name
}

// isExecutable reports whether path is a regular file with an execute bit set.
func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
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
