package config

import (
	"strconv"

	"github.com/spf13/pflag"
)

// KeyWorkers is the workers flag name.
const KeyWorkers = "workers"

// DefaultWorkers is one, since each encode already uses every core or the media engine.
//
//nolint:gochecknoglobals // Flag definition.
var DefaultWorkers = 1

// FlagWorkers sets how many jobs run at once.
//
//nolint:gochecknoglobals // Flag definition.
var FlagWorkers = pflag.Flag{
	Name:     KeyWorkers,
	Usage:    "Jobs to run at once.",
	Value:    &FlagValue{Val: strconv.Itoa(DefaultWorkers), ValType: "int"},
	DefValue: strconv.Itoa(DefaultWorkers),
}

// Workers returns the configured worker count.
func Workers() int { return loadInt(KeyWorkers, DefaultWorkers) }
