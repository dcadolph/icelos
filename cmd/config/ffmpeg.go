package config

import (
	"github.com/spf13/pflag"
)

// KeyFFmpeg is the ffmpeg flag name.
const KeyFFmpeg = "ffmpeg"

// DefaultFFmpeg is empty, meaning auto-detect.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultFFmpeg = ""

// FlagFFmpeg sets the ffmpeg binary.
//
//nolint:gochecknoglobals // Flag definition.
var FlagFFmpeg = pflag.Flag{
	Name:     KeyFFmpeg,
	Usage:    "Path to ffmpeg. Defaults to the copy beside icelos, then PATH.",
	Value:    &FlagValue{Val: DefaultFFmpeg, ValType: "string"},
	DefValue: DefaultFFmpeg,
}

// FFmpeg returns the configured ffmpeg path.
func FFmpeg() string { return loadString(KeyFFmpeg, DefaultFFmpeg) }
