package cmd

import "runtime/debug"

// version is stamped at build time via ldflags:
// -ldflags "-X github.com/dcadolph/icelos/cmd.version=v1.2.3".
//
//nolint:gochecknoglobals // Build-time stamp.
var version = "dev"

// resolveVersion returns the stamped build version, falling back to the module version when
// the binary was built with go install.
func resolveVersion() string {
	if version != "dev" && version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
