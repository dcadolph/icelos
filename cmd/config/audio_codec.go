package config

import (
	"github.com/spf13/pflag"
)

// KeyAudioCodec is the audio-codec flag name.
const KeyAudioCodec = "audio-codec"

// DefaultAudioCodec is empty, so the preset's audio codec is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultAudioCodec = ""

// FlagAudioCodec sets the audio codec.
//
//nolint:gochecknoglobals // Flag definition.
var FlagAudioCodec = pflag.Flag{
	Name:     KeyAudioCodec,
	Usage:    "Audio codec ID, copy to pass the audio through, or none for silent output.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// AudioCodec returns the raw audio-codec value.
func AudioCodec() string { return loadString(KeyAudioCodec, DefaultAudioCodec) }
