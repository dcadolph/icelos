package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// Execute runs the root command and exits with the matching code.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := rootCmd().ExecuteContext(ctx)
	stop()
	switch {
	case err == nil:
	case errors.Is(err, errInterrupted):
		os.Exit(codeInterrupted)
	case errors.Is(err, errJobsFailed):
		fmt.Fprintln(os.Stderr, "icelos:", err)
		os.Exit(codeFailed)
	default:
		fmt.Fprintln(os.Stderr, "icelos:", err)
		os.Exit(codeError)
	}
}
