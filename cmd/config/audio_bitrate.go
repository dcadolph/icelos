package config

import (
	"github.com/spf13/pflag"
)

// KeyAudioBitrate is the audio-bitrate flag name.
const KeyAudioBitrate = "audio-bitrate"

// DefaultAudioBitrate is empty, so the preset's bitrate is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultAudioBitrate = ""

// FlagAudioBitrate sets the audio bitrate.
//
//nolint:gochecknoglobals // Flag definition.
var FlagAudioBitrate = pflag.Flag{
	Name:     KeyAudioBitrate,
	Usage:    "Audio bitrate in bits per second with optional k or M suffix, e.g. 128k.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// AudioBitrate returns the raw audio-bitrate value.
func AudioBitrate() string { return loadString(KeyAudioBitrate, DefaultAudioBitrate) }
