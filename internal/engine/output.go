package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxNameTries bounds the search for a free output name.
const maxNameTries = 999

// OutputPath returns a free path for converting input to a file ending in ext. The file goes
// in dir, or next to input when dir is empty. Names on disk or reported by taken get a
// Finder-style number, e.g. "clip 2.mp4", and the input itself is never chosen. A nil taken
// checks the disk only.
func OutputPath(input, ext, dir string, taken func(string) bool) (string, error) {
	in, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrOutput, err)
	}
	if dir == "" {
		dir = filepath.Dir(in)
	}
	base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
	for n := 1; n <= maxNameTries; n++ {
		name := base
		if n > 1 {
			name += " " + strconv.Itoa(n)
		}
		candidate, err := filepath.Abs(filepath.Join(dir, name+"."+ext))
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrOutput, err)
		}
		if candidate == in || (taken != nil && taken(candidate)) {
			continue
		}
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: no free name for %s in %s", ErrExists, base, dir)
}
