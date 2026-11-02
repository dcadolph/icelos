package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dcadolph/icelos/internal/preset"
)

// presetsCmd builds the presets subcommand.
func presetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "presets",
		Short: "List the built-in presets as JSON.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeJSON(cmd.OutOrStdout(), preset.All())
		},
	}
}
