package config

import (
	"github.com/spf13/pflag"
)

// KeySize is the size flag name.
const KeySize = "size"

// DefaultSize is empty, meaning constant-quality output.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultSize = ""

// FlagSize sets the target size.
//
//nolint:gochecknoglobals // Flag definition.
var FlagSize = pflag.Flag{
	Name:     KeySize,
	Usage:    "Target output size for the target-size preset, e.g. 25MB or 8MiB.",
	Value:    &FlagValue{Val: DefaultSize, ValType: "string"},
	DefValue: DefaultSize,
}

// Size returns the raw target size string.
func Size() string { return loadString(KeySize, DefaultSize) }
