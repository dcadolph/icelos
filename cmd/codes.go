package cmd

// Exit codes for the icelos binary.
const (
	// codeFailed means at least one job failed.
	codeFailed = 1
	// codeError means a usage or setup problem stopped the run.
	codeError = 2
	// codeInterrupted means the user pressed Ctrl+C.
	codeInterrupted = 130
)
