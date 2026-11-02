package cmd

import "errors"

var (
	// errJobsFailed is returned when convert finishes with failed jobs.
	errJobsFailed = errors.New("some jobs failed")
	// errInterrupted is returned when the user stops a run. It exits quietly.
	errInterrupted = errors.New("interrupted")
	// errUsage is returned for flag combinations that make no sense.
	errUsage = errors.New("usage")
)
