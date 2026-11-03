package preset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

const (
	// recentFile holds the recently used preset IDs inside the store folder.
	recentFile = "recent.json"
	// recentMax is how many recently used IDs the store keeps.
	recentMax = 10
)

// idPattern is the shape of a custom preset ID, which is also its file name.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Store keeps custom presets as one JSON file each in a folder, beside a list of the presets
// used most recently. The app and the command line read the same folder.
type Store struct {
	// dir is the folder holding the preset files and the recent list.
	dir string
	// mu serializes reads and writes.
	mu sync.Mutex
}

// NewStore returns a Store in icelos/presets under the user's configuration folder, which is
// ~/Library/Application Support on macOS. The folder is created on the first save.
func NewStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStore, err)
	}
	return WithDir(filepath.Join(base, "icelos", "presets")), nil
}

// WithDir returns a Store in dir.
func WithDir(dir string) *Store {
	if dir == "" {
		panic("preset.WithDir: dir required")
	}
	return &Store{dir: dir}
}

// Dir returns the folder the store reads and writes.
func (s *Store) Dir() string {
	return s.dir
}

// List returns the custom presets sorted by name, each marked Custom. A file that is not a valid
// preset is left out and named in the returned error, so one bad file never hides the rest.
func (s *Store) List() ([]Preset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStore, err)
	}
	var out []Preset
	var errs []error
	for _, e := range entries {
		name := e.Name()
		id, ok := strings.CutSuffix(name, ".json")
		if e.IsDir() || !ok || name == recentFile || !idPattern.MatchString(id) {
			continue
		}
		p, err := s.read(id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Preset) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, errors.Join(errs...)
}

// Get returns the custom preset with the given ID.
func (s *Store) Get(id string) (Preset, error) {
	if !idPattern.MatchString(id) {
		return Preset{}, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// Lookup returns the built-in preset with the given ID, or else the custom one.
func (s *Store) Lookup(id string) (Preset, error) {
	if p, err := Lookup(id); err == nil {
		return p, nil
	}
	return s.Get(id)
}

// Save validates p and writes it as a custom preset, replacing any custom preset with its ID.
// Built-in IDs are refused.
func (s *Store) Save(p Preset) error {
	if err := checkID(p.ID); err != nil {
		return err
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%w: %s: name required", ErrInvalid, p.ID)
	}
	if err := p.Validate(); err != nil {
		return err
	}
	p.Custom = false
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(p.ID+".json", data)
}

// Delete removes the custom preset with the given ID and drops it from the recent list.
func (s *Store) Delete(id string) error {
	if err := checkID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(filepath.Join(s.dir, id+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
	}
	recent, err := s.recent()
	if err != nil || !slices.Contains(recent, id) {
		return err
	}
	return s.writeRecent(slices.DeleteFunc(recent, func(r string) bool { return r == id }))
}

// Recent returns the IDs of the presets used most recently, newest first.
func (s *Store) Recent() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recent()
}

// Use moves id to the front of the recent list, keeping the newest ten.
func (s *Store) Use(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recent, err := s.recent()
	if err != nil {
		return err
	}
	recent = slices.DeleteFunc(recent, func(r string) bool { return r == id })
	recent = append([]string{id}, recent...)
	return s.writeRecent(recent[:min(len(recent), recentMax)])
}

// checkID reports whether id can name a custom preset.
func checkID(id string) error {
	switch {
	case slices.ContainsFunc(builtin, func(p Preset) bool { return p.ID == id }):
		return fmt.Errorf("%w: %q", ErrBuiltin, id)
	case !idPattern.MatchString(id), id+".json" == recentFile:
		return fmt.Errorf("%w: id %q must be lowercase letters, digits, dashes, or underscores",
			ErrInvalid, id)
	}
	return nil
}

// read loads the custom preset with the given ID. The caller holds mu.
func (s *Store) read(id string) (Preset, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return Preset{}, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	if err != nil {
		return Preset{}, fmt.Errorf("%w: %w", ErrStore, err)
	}
	var p Preset
	if err := json.Unmarshal(data, &p); err != nil {
		return Preset{}, fmt.Errorf("%w: %s.json: %w", ErrStore, id, err)
	}
	if p.ID != id {
		return Preset{}, fmt.Errorf("%w: %s.json holds id %q", ErrStore, id, p.ID)
	}
	if err := p.Validate(); err != nil {
		return Preset{}, fmt.Errorf("%w: %s.json: %w", ErrStore, id, err)
	}
	p.Custom = true
	return p, nil
}

// recent loads the recent list, empty when it does not exist yet. The caller holds mu.
func (s *Store) recent() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, recentFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStore, err)
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrStore, recentFile, err)
	}
	return ids, nil
}

// writeRecent replaces the recent list. The caller holds mu.
func (s *Store) writeRecent(ids []string) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
	}
	return s.write(recentFile, data)
}

// write replaces the named file in the store folder through a temporary file and a rename, so a
// reader never sees half a file. The caller holds mu.
func (s *Store) write(name string, data []byte) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
	}
	f, err := os.CreateTemp(s.dir, "."+name+".*.tmp")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStore, err)
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("%w: %w", ErrStore, err)
	}
	if err := os.Rename(f.Name(), filepath.Join(s.dir, name)); err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("%w: %w", ErrStore, err)
	}
	return nil
}
