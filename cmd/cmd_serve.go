package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dcadolph/icelos/cmd/config"
	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/serve"
)

// serveCmd builds the serve subcommand the app runs as its backend.
func serveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Speak the JSON-lines protocol on stdin and stdout.",
		Long: `serve reads one JSON request per line on stdin and writes one JSON reply or event
per line on stdout. The app runs it as a subprocess. Closing stdin cancels running jobs
and exits.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enc, prober := newEngine()
			store, err := preset.NewStore()
			if err != nil {
				return err
			}
			cache, err := os.UserCacheDir()
			if err != nil {
				return fmt.Errorf("thumbnail cache: %w", err)
			}
			thumbs := filepath.Join(cache, "icelos", "thumbs")
			thumbnail := func(ctx context.Context, r engine.ThumbnailRequest) (string, error) {
				return enc.Thumbnail(ctx, thumbs, r)
			}
			srv := serve.New(cmd.OutOrStdout(), serve.Config{Encoder: enc, Prober: prober,
				Encoders: enc.Encoders, Thumbnail: thumbnail, Presets: store, Version: resolveVersion(),
				Workers: config.Workers()})
			return srv.Run(cmd.Context(), cmd.InOrStdin())
		},
	}
	cmd.Flags().AddFlag(config.Lookup(config.KeyWorkers))
	return cmd
}
