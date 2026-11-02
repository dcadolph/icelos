package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyHardware is the hardware flag name.
const KeyHardware = "hardware"

// DefaultHardware is true, since the media engine is fast and frees the CPU.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultHardware = true

// FlagHardware toggles hardware encoding.
//
//nolint:gochecknoglobals // Flag definition.
var FlagHardware = pflag.Flag{
	Name:        KeyHardware,
	Usage:       "Prefer the VideoToolbox hardware encoders, falling back to software.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultHardware), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultHardware),
	NoOptDefVal: "true",
}

// Hardware reports whether hardware encoding is preferred.
func Hardware() bool { return loadBool(KeyHardware, DefaultHardware) }
