package config

import (
	"github.com/spf13/pflag"
)

// KeyResolution is the resolution flag name.
const KeyResolution = "resolution"

// DefaultResolution is empty, so the preset's resolution is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultResolution = ""

// FlagResolution sets the output resolution.
//
//nolint:gochecknoglobals // Flag definition.
var FlagResolution = pflag.Flag{
	Name:     KeyResolution,
	Usage:    "Resolution ID such as 1080p or same, or exact WxH such as 1280x720.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// Resolution returns the raw resolution value.
func Resolution() string { return loadString(KeyResolution, DefaultResolution) }
