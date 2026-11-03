package config

import (
	"github.com/spf13/pflag"
)

// KeyPassthrough is the passthrough flag name.
const KeyPassthrough = "passthrough"

// DefaultPassthrough is false.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultPassthrough = false

// FlagPassthrough copies matching streams.
//
//nolint:gochecknoglobals // Flag definition.
var FlagPassthrough = pflag.Flag{
	Name:        KeyPassthrough,
	Usage:       "Copy streams that already match the preset instead of encoding them.",
	Value:       &FlagValue{Val: "false", ValType: "bool"},
	DefValue:    "false",
	NoOptDefVal: "true",
}

// Passthrough returns the raw passthrough value.
func Passthrough() bool { return loadBool(KeyPassthrough, DefaultPassthrough) }
