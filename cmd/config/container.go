package config

import (
	"github.com/spf13/pflag"
)

// KeyContainer is the container flag name.
const KeyContainer = "container"

// DefaultContainer is empty, so the preset's container is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultContainer = ""

// FlagContainer sets the output container.
//
//nolint:gochecknoglobals // Flag definition.
var FlagContainer = pflag.Flag{
	Name:     KeyContainer,
	Usage:    "Output container ID, e.g. mp4, mkv, or mov.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// Container returns the raw container value.
func Container() string { return loadString(KeyContainer, DefaultContainer) }
