package engine

import (
	"bufio"
	"bytes"
	"strings"
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
