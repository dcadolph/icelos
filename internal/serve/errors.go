package serve

import "errors"

var (
	// ErrRequest is returned for a request line that cannot be read or is incomplete.
	ErrRequest = errors.New("bad request")
	// ErrUnknownOp is returned for a request with an unrecognized op.
	ErrUnknownOp = errors.New("unknown op")
	// ErrWrite is returned when the output cannot be written.
	ErrWrite = errors.New("write failed")
)
