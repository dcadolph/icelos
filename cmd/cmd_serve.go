package cmd

import (
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
			// Replanning after a settings change reuses the probe of an unchanged file.
			prober = engine.NewProbeCache(prober)
			store, err := preset.NewStore()
			if err != nil {
				return err
			}
			srv := serve.New(cmd.OutOrStdout(), serve.Config{Encoder: enc, Prober: prober,
				Encoders: enc.Encoders, Presets: store, Version: resolveVersion(),
				Workers: config.Workers()})
			return srv.Run(cmd.Context(), cmd.InOrStdin())
		},
	}
	cmd.Flags().AddFlag(config.Lookup(config.KeyWorkers))
	return cmd
}
