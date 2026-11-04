package engine

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/dcadolph/icelos/internal/probe"
)

// probeCacheSize bounds how many probe results a ProbeCache keeps.
const probeCacheSize = 256

// ProbeCache is a Prober that keeps results by absolute path, size, and modification time, so a
// second question about an unchanged file costs a stat instead of an ffprobe run. Failures are
// not kept, and the oldest result goes first once the cache is full. Results share their slices
// between callers, which treat probe results as read-only. It is safe for concurrent use.
type ProbeCache struct {
	// prober reads files the cache does not hold.
	prober Prober
	// mu guards entries and order.
	mu sync.Mutex
	// entries holds results by file version.
	entries map[probeKey]probe.Info
	// order lists the keys in entries oldest first.
	order []probeKey
}

// probeKey identifies one version of a file.
type probeKey struct {
	// Path is the absolute path.
	Path string
	// Size is the size in bytes.
	Size int64
	// ModTime is the modification time in nanoseconds since the Unix epoch.
	ModTime int64
}

// NewProbeCache returns a ProbeCache in front of p. It panics when p is nil.
func NewProbeCache(p Prober) *ProbeCache {
	if p == nil {
		panic("engine.NewProbeCache: Prober required")
	}
	return &ProbeCache{prober: p, entries: make(map[probeKey]probe.Info)}
}

// Probe returns the kept result for path when the file has not changed since it was read, and
// otherwise probes it and keeps the result. A path that cannot be stated goes to the wrapped
// Prober every time, so its error is the Prober's own.
func (c *ProbeCache) Probe(ctx context.Context, path string) (probe.Info, error) {
	key, ok := probeKeyOf(path)
	if !ok {
		return c.prober.Probe(ctx, path)
	}
	c.mu.Lock()
	info, hit := c.entries[key]
	c.mu.Unlock()
	if hit {
		return info, nil
	}
	info, err := c.prober.Probe(ctx, path)
	if err != nil {
		return info, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, dup := c.entries[key]; !dup {
		if len(c.order) == probeCacheSize {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
		c.entries[key] = info
		c.order = append(c.order, key)
	}
	return info, nil
}

// probeKeyOf returns the key of the file at path as it is now, and false when path is not a
// regular file that can be stated.
func probeKeyOf(path string) (probeKey, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return probeKey{}, false
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return probeKey{}, false
	}
	return probeKey{Path: abs, Size: fi.Size(), ModTime: fi.ModTime().UnixNano()}, true
}
