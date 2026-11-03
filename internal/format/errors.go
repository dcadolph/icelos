package format

import "errors"

var (
	// ErrUnknownContainer is returned when no container has the requested ID.
	ErrUnknownContainer = errors.New("unknown container")
	// ErrUnknownCodec is returned when no codec has the requested ID.
	ErrUnknownCodec = errors.New("unknown codec")
	// ErrMode is returned when a frame shape fits none of a fixed-mode codec's or container's modes.
	ErrMode = errors.New("frame shape not allowed")
	// ErrSetting is returned when a target asks for a setting its container or codec refuses.
	ErrSetting = errors.New("setting not allowed")
)
