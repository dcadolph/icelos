package engine

import (
	"bufio"
	"bytes"
	"slices"
	"strings"

	"github.com/dcadolph/icelos/internal/format"
)

// EncoderSet is the set of encoder names an ffmpeg build offers.
type EncoderSet map[string]bool

// Has reports whether name is in the set.
func (s EncoderSet) Has(name string) bool {
	return s[name]
}

// ParseEncoders reads the output of "ffmpeg -hide_banner -encoders".
func ParseEncoders(out []byte) EncoderSet {
	set := EncoderSet{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	listing := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !listing {
			listing = strings.HasPrefix(line, "---")
			continue
		}
		if fields := strings.Fields(line); len(fields) >= 2 && len(fields[0]) == 6 {
			set[fields[1]] = true
		}
	}
	return set
}

// Drives reports whether icelos has an argument builder for the named encoder.
func Drives(name string) bool {
	_, video := videoEncoders[name]
	_, audio := audioEncoders[name]
	return video || audio
}

// Usable returns the sorted names of the catalog encoders that set offers and icelos can drive.
func Usable(set EncoderSet) []string {
	var out []string
	for _, c := range format.Codecs() {
		for _, e := range c.Encoders {
			if set.Has(e.Name) && Drives(e.Name) && !slices.Contains(out, e.Name) {
				out = append(out, e.Name)
			}
		}
	}
	slices.Sort(out)
	return out
}

// Available reports whether set offers an encoder for codec that icelos can drive.
func Available(codec format.Codec, set EncoderSet) bool {
	return slices.ContainsFunc(codec.Encoders, func(e format.Encoder) bool {
		return set.Has(e.Name) && Drives(e.Name)
	})
}
