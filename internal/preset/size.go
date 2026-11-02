package preset

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// sizeUnits maps lowercase unit suffixes to their byte multiplier, longest suffix first.
var sizeUnits = []struct { //nolint:gochecknoglobals // Read-only table.
	// Suffix is the unit as typed, lowercase.
	Suffix string
	// Bytes is the size of one unit.
	Bytes float64
}{
	{"gib", 1 << 30}, {"mib", 1 << 20}, {"kib", 1 << 10},
	{"gb", 1e9}, {"mb", 1e6}, {"kb", 1e3},
	{"g", 1e9}, {"m", 1e6}, {"k", 1e3}, {"b", 1},
}

// ParseSize reads a human size such as "25MB", "8 MiB", or "1.5g" into bytes.
// A bare number is read as megabytes, since that is how people state upload limits.
func ParseSize(s string) (int64, error) {
	in := strings.ToLower(strings.TrimSpace(s))
	mult := 1e6
	for _, u := range sizeUnits {
		if num, ok := strings.CutSuffix(in, u.Suffix); ok {
			in, mult = strings.TrimSpace(num), u.Bytes
			break
		}
	}
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrSize, s)
	}
	n := f * mult
	if n < 1 || math.IsInf(n, 0) || math.IsNaN(n) || n > math.MaxInt64/2 {
		return 0, fmt.Errorf("%w: %q out of range", ErrSize, s)
	}
	return int64(n), nil
}
