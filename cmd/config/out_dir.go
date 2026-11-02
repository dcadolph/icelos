package config

import (
	"github.com/spf13/pflag"
)

// KeyOutDir is the out-dir flag name.
const KeyOutDir = "out-dir"

// DefaultOutDir is empty, meaning next to each input.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultOutDir = ""

// FlagOutDir sets the output folder.
//
//nolint:gochecknoglobals // Flag definition.
var FlagOutDir = pflag.Flag{
	Name:     KeyOutDir,
	Usage:    "Folder for outputs. Defaults to the folder of each input.",
	Value:    &FlagValue{Val: DefaultOutDir, ValType: "string"},
	DefValue: DefaultOutDir,
}

// OutDir returns the configured output folder.
func OutDir() string { return loadString(KeyOutDir, DefaultOutDir) }
