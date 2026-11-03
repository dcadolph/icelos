package config

import (
	"github.com/spf13/pflag"
)

// KeyFPS is the fps flag name.
const KeyFPS = "fps"

// DefaultFPS is empty, so the preset's frame rate is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultFPS = ""

// FlagFPS sets the frame rate.
//
//nolint:gochecknoglobals // Flag definition.
var FlagFPS = pflag.Flag{
	Name:     KeyFPS,
	Usage:    "Frame rate, e.g. 30 or 23.976, 0 for the source rate.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// FPS returns the raw fps value.
func FPS() string { return loadString(KeyFPS, DefaultFPS) }
