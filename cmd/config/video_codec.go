package config

import (
	"github.com/spf13/pflag"
)

// KeyVideoCodec is the video-codec flag name.
const KeyVideoCodec = "video-codec"

// DefaultVideoCodec is empty, so the preset's video codec is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultVideoCodec = ""

// FlagVideoCodec sets the video codec.
//
//nolint:gochecknoglobals // Flag definition.
var FlagVideoCodec = pflag.Flag{
	Name:     KeyVideoCodec,
	Usage:    "Video codec ID, or copy to pass the video through.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// VideoCodec returns the raw video-codec value.
func VideoCodec() string { return loadString(KeyVideoCodec, DefaultVideoCodec) }
