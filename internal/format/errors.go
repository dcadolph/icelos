package format

import "errors"

var (
	// ErrUnknownContainer is returned when no container has the requested ID.
	ErrUnknownContainer = errors.New("unknown container")
	// ErrUnknownCodec is returned when no codec has the requested ID.
	ErrUnknownCodec = errors.New("unknown codec")
)
