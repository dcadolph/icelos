package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyOverwrite is the overwrite flag name.
const KeyOverwrite = "overwrite"

// DefaultOverwrite is false, so existing files are never replaced.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultOverwrite = false

// FlagOverwrite allows replacing existing outputs.
//
//nolint:gochecknoglobals // Flag definition.
var FlagOverwrite = pflag.Flag{
	Name:        KeyOverwrite,
	Usage:       "Replace an existing output file.",
	Value:       &FlagValue{Val: strconv.FormatBool(DefaultOverwrite), ValType: "bool"},
	DefValue:    strconv.FormatBool(DefaultOverwrite),
	NoOptDefVal: "true",
}

// Overwrite reports whether outputs may be replaced.
func Overwrite() bool { return loadBool(KeyOverwrite, DefaultOverwrite) }
