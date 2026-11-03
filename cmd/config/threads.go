package config

import (
	"github.com/spf13/pflag"
)

// KeyThreads is the threads flag name.
const KeyThreads = "threads"

// DefaultThreads is zero, meaning ffmpeg's default.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultThreads = 0

// FlagThreads caps the encoder threads.
//
//nolint:gochecknoglobals // Flag definition.
var FlagThreads = pflag.Flag{
	Name:     KeyThreads,
	Usage:    "Encoder thread cap, 0 for ffmpeg's default.",
	Value:    &FlagValue{Val: "0", ValType: "int"},
	DefValue: "0",
}

// Threads returns the raw threads value.
func Threads() int { return loadInt(KeyThreads, DefaultThreads) }
