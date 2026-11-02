// Package progress parses the ffmpeg -progress key=value stream.
package progress

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"
)

// Update is one progress report from ffmpeg.
type Update struct {
	// Elapsed is the media time encoded so far.
	Elapsed time.Duration
	// Speed is the encode speed as a multiple of real time.
	Speed float64
	// FPS is the current encoding frame rate.
	FPS float64
	// Done is true on the final report.
	Done bool
}

// Read parses r and calls fn once per completed report until r ends.
func Read(r io.Reader, fn func(Update)) error {
	var cur Update
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "out_time_us", "out_time_ms":
			if us, err := strconv.ParseInt(val, 10, 64); err == nil && us >= 0 {
				cur.Elapsed = time.Duration(us) * time.Microsecond
			}
		case "speed":
			if f, err := strconv.ParseFloat(strings.TrimSuffix(val, "x"), 64); err == nil {
				cur.Speed = f
			}
		case "fps":
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				cur.FPS = f
			}
		case "progress":
			cur.Done = val == "end"
			fn(cur)
		}
	}
	return sc.Err()
}
