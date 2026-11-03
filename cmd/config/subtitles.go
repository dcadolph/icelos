package config

import (
	"github.com/spf13/pflag"
)

// KeySubtitles is the subtitles flag name.
const KeySubtitles = "subtitles"

// DefaultSubtitles is empty, so subtitles are left out.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultSubtitles = ""

// FlagSubtitles chooses the subtitle handling.
//
//nolint:gochecknoglobals // Flag definition.
var FlagSubtitles = pflag.Flag{
	Name:     KeySubtitles,
	Usage:    "Subtitles: keep, none, burn, or the path of a subtitle file to add.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// Subtitles returns the raw subtitles value.
func Subtitles() string { return loadString(KeySubtitles, DefaultSubtitles) }
