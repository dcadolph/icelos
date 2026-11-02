package cmd

import (
	"slices"

	"github.com/spf13/cobra"
)

// encoderReport is the encoders subcommand output.
type encoderReport struct {
	// FFmpeg is the ffmpeg binary that was asked.
	FFmpeg string `json:"ffmpeg"`
	// Hardware lists the VideoToolbox encoders present.
	Hardware []string `json:"hardware"`
	// Software lists the software encoders icelos uses that are present.
	Software []string `json:"software"`
}

// encodersCmd builds the encoders subcommand.
func encodersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "encoders",
		Short: "Show which encoders icelos can use with this ffmpeg.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enc, _ := newEngine()
			set, err := enc.Encoders(cmd.Context())
			if err != nil {
				return err
			}
			rep := encoderReport{FFmpeg: enc.Bin(), Hardware: []string{}, Software: []string{}}
			for _, name := range []string{"h264_videotoolbox", "hevc_videotoolbox"} {
				if set.Has(name) {
					rep.Hardware = append(rep.Hardware, name)
				}
			}
			for _, name := range []string{"libx264", "libx265", "aac", "aac_at", "libmp3lame"} {
				if set.Has(name) {
					rep.Software = append(rep.Software, name)
				}
			}
			slices.Sort(rep.Software)
			return writeJSON(cmd.OutOrStdout(), rep)
		},
	}
}
