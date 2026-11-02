package config

import (
	"github.com/spf13/pflag"
)

// KeyPreset is the preset flag name.
const KeyPreset = "preset"

// DefaultPreset is apple, the HEVC preset for Apple devices.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultPreset = "apple"

// FlagPreset selects the conversion preset.
//
//nolint:gochecknoglobals // Flag definition.
var FlagPreset = pflag.Flag{
	Name:     KeyPreset,
	Usage:    "Preset ID, see the presets command.",
	Value:    &FlagValue{Val: DefaultPreset, ValType: "string"},
	DefValue: DefaultPreset,
}

// Preset returns the configured preset ID.
func Preset() string { return loadString(KeyPreset, DefaultPreset) }
