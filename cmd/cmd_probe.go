package cmd

import (
	"github.com/spf13/cobra"
)

// probeCmd builds the probe subcommand.
func probeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "probe FILE",
		Short: "Print a file's container and streams as JSON.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, prober := newEngine()
			info, err := prober.Probe(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), info)
		},
	}
}
