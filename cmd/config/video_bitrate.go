package config

import (
	"github.com/spf13/pflag"
)

// KeyVideoBitrate is the video-bitrate flag name.
const KeyVideoBitrate = "video-bitrate"

// DefaultVideoBitrate is empty, so the preset's bitrate is used.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultVideoBitrate = ""

// FlagVideoBitrate sets the video bitrate.
//
//nolint:gochecknoglobals // Flag definition.
var FlagVideoBitrate = pflag.Flag{
	Name:     KeyVideoBitrate,
	Usage:    "Video bitrate in bits per second with optional k or M suffix, e.g. 5M.",
	Value:    &FlagValue{Val: "", ValType: "string"},
	DefValue: "",
}

// VideoBitrate returns the raw video-bitrate value.
func VideoBitrate() string { return loadString(KeyVideoBitrate, DefaultVideoBitrate) }
