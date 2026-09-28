package buildinfo

import (
	"runtime"
	"runtime/debug"
)

// Build is what the binary was built with, and from.
type Build struct {
	// Version is what Version returns.
	Version string
	// Go is the version of Go, such as go1.26.0.
	Go string
	// Revision, Time and Modified tell the commit of the source, when it
	// was made, and whether the source had changes beside it. They are
	// empty for a build that didn't record them, as go install's doesn't.
	Revision string
	Time     string
	Modified bool
	// CGO is the CGO_ENABLED of the build, "1" or "0", or "" if unknown.
	CGO string
}

// Read returns what the binary was built with.
func Read() Build {
	b := Build{Version: Version(), Go: runtime.Version()}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision = s.Value
		case "vcs.time":
			b.Time = s.Value
		case "vcs.modified":
			b.Modified = s.Value == "true"
		case "CGO_ENABLED":
			b.CGO = s.Value
		}
	}
	return b
}
