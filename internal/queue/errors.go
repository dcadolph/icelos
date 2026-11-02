package queue

import "errors"

var (
	// ErrInput is returned when a job's input file cannot be used.
	ErrInput = errors.New("bad input")
	// ErrUnknownJob is returned when no job has the requested ID.
	ErrUnknownJob = errors.New("unknown job")
	// ErrDuplicateOutput is returned when two unfinished jobs would write the same file.
	ErrDuplicateOutput = errors.New("output already claimed")
)
