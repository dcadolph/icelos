package config

import "github.com/spf13/pflag"

// flags indexes every flag by key so lookups and resets can walk them.
//
//nolint:gochecknoglobals // Flag registry, written once at init.
var flags = map[string]*pflag.Flag{
	KeyPreset:    &FlagPreset,
	KeySize:      &FlagSize,
	KeyOutDir:    &FlagOutDir,
	KeyOutput:    &FlagOutput,
	KeyHardware:  &FlagHardware,
	KeyOverwrite: &FlagOverwrite,
	KeyWorkers:   &FlagWorkers,
	KeyPretty:    &FlagPretty,
	KeyFFmpeg:    &FlagFFmpeg,
	KeyFFprobe:   &FlagFFprobe,
}

// Reset puts every flag back to its default. Tests use it between command runs because flag
// state is shared package-wide.
func Reset() {
	for _, f := range flags {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
}

// Lookup returns the flag for key. It panics on an unknown key, which is a programming error.
func Lookup(key string) *pflag.Flag {
	f, ok := flags[key]
	if !ok {
		panic("config.Lookup: unknown flag " + key)
	}
	return f
}
