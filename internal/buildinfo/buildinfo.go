// Package buildinfo tells what gh-tui was built from.
package buildinfo

import "runtime/debug"

// Version returns the version of the module the binary was built from,
// such as v1.2.3, or "(devel)" for a build with none, or "" when the
// binary carries no build information.
func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return ""
}
