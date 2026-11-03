package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dcadolph/icelos/internal/engine"
	"github.com/dcadolph/icelos/internal/format"
)

// encoderReport is the encoders subcommand output.
type encoderReport struct {
	// FFmpeg is the ffmpeg binary that was asked.
	FFmpeg string `json:"ffmpeg"`
	// Hardware lists the VideoToolbox encoders present that icelos can drive.
	Hardware []string `json:"hardware"`
	// Software lists the software encoders present that icelos can drive.
	Software []string `json:"software"`
	// Codecs says, for each catalog codec, whether this ffmpeg can encode it.
	Codecs []codecReport `json:"codecs"`
}

// codecReport is one catalog codec's availability.
type codecReport struct {
	// ID is the catalog codec ID.
	ID string `json:"id"`
	// Kind is the stream kind the codec produces.
	Kind format.Kind `json:"kind"`
	// Available is true when ffmpeg offers an encoder for the codec that icelos can drive.
	Available bool `json:"available"`
}

// encodersCmd builds the encoders subcommand.
func encodersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "encoders",
		Short: "Show which encoders and codecs icelos can use with this ffmpeg.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enc, _ := newEngine()
			set, err := enc.Encoders(cmd.Context())
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), newEncoderReport(enc.Bin(), set))
		},
	}
}

// newEncoderReport describes what set lets icelos encode.
func newEncoderReport(bin string, set engine.EncoderSet) encoderReport {
	hardware := map[string]bool{}
	rep := encoderReport{FFmpeg: bin, Hardware: []string{}, Software: []string{}}
	for _, c := range format.Codecs() {
		for _, e := range c.Encoders {
			hardware[e.Name] = hardware[e.Name] || e.Hardware
		}
		rep.Codecs = append(rep.Codecs, codecReport{ID: c.ID, Kind: c.Kind,
			Available: engine.Available(c, set)})
	}
	for _, name := range engine.Usable(set) {
		if hardware[name] {
			rep.Hardware = append(rep.Hardware, name)
		} else {
			rep.Software = append(rep.Software, name)
		}
	}
	return rep
}
