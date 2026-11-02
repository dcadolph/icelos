package config

import (
	"github.com/spf13/pflag"
)

// KeyFFprobe is the ffprobe flag name.
const KeyFFprobe = "ffprobe"

// DefaultFFprobe is empty, meaning auto-detect.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultFFprobe = ""

// FlagFFprobe sets the ffprobe binary.
//
//nolint:gochecknoglobals // Flag definition.
var FlagFFprobe = pflag.Flag{
	Name:     KeyFFprobe,
	Usage:    "Path to ffprobe. Defaults to the copy beside icelos, then PATH.",
	Value:    &FlagValue{Val: DefaultFFprobe, ValType: "string"},
	DefValue: DefaultFFprobe,
}

// FFprobe returns the configured ffprobe path.
func FFprobe() string { return loadString(KeyFFprobe, DefaultFFprobe) }
