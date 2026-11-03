package config

import (
	"github.com/spf13/pflag"
)

// KeyAudioTracks is the audio-tracks flag name.
const KeyAudioTracks = "audio-tracks"

// DefaultAudioTracks is empty, so the first audio track is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultAudioTracks = ""

// FlagAudioTracks chooses the audio tracks.
//
//nolint:gochecknoglobals // Flag definition.
var FlagAudioTracks = pflag.Flag{
	Name:     KeyAudioTracks,
	Usage:    "Audio tracks to keep: all, or a comma list of track numbers from 1, e.g. 1,2.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// AudioTracks returns the raw audio-tracks value.
func AudioTracks() string { return loadString(KeyAudioTracks, DefaultAudioTracks) }
