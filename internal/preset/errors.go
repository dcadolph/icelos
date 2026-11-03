package preset

import "errors"

var (
	// ErrUnknown is returned when no preset has the requested ID.
	ErrUnknown = errors.New("unknown preset")
	// ErrInvalid is returned when a preset contradicts itself.
	ErrInvalid = errors.New("invalid preset")
	// ErrSize is returned when a size string cannot be read.
	ErrSize = errors.New("invalid size")
	// ErrBuiltin is returned when a custom preset would replace or remove a built-in one.
	ErrBuiltin = errors.New("built-in preset")
	// ErrStore is returned when the custom preset folder cannot be read or written.
	ErrStore = errors.New("preset store")
)
