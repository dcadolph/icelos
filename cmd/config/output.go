package config

import (
	"github.com/spf13/pflag"
)

// KeyOutput is the output flag name.
const KeyOutput = "output"

// DefaultOutput is empty, meaning a free name is chosen.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultOutput = ""

// FlagOutput sets an exact output path.
//
//nolint:gochecknoglobals // Flag definition.
var FlagOutput = pflag.Flag{
	Name:     KeyOutput,
	Usage:    "Exact output path. Only valid with a single input.",
	Value:    &FlagValue{Val: DefaultOutput, ValType: "string"},
	DefValue: DefaultOutput,
}

// Output returns the configured output path.
func Output() string { return loadString(KeyOutput, DefaultOutput) }
