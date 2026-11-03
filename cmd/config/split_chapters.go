package config

import (
	"github.com/spf13/pflag"
)

// KeySplitChapters is the split-chapters flag name.
const KeySplitChapters = "split-chapters"

// DefaultSplitChapters is false.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultSplitChapters = false

// FlagSplitChapters adds one job per chapter.
//
//nolint:gochecknoglobals // Flag definition.
var FlagSplitChapters = pflag.Flag{
	Name:        KeySplitChapters,
	Usage:       "Convert each chapter of the input to its own file.",
	Value:       &FlagValue{Val: "false", ValType: "bool"},
	DefValue:    "false",
	NoOptDefVal: "true",
}

// SplitChapters returns the raw split-chapters value.
func SplitChapters() bool { return loadBool(KeySplitChapters, DefaultSplitChapters) }
