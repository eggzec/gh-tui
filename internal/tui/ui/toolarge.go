package ui

import (
	"errors"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// TooLargeText says why the file name is refused as too large, from the size
// and limit err carries, and names the setting of the limit.
func TooLargeText(name string, err error) string {
	var tl *core.TooLargeError
	if !errors.As(err, &tl) {
		return name + " is too large."
	}
	if tl.Warn {
		return config.WarnText(name, tl.Size, tl.Limit)
	}
	return config.TooLargeText(name, tl.Size, tl.Limit)
}

// OpenToLoadText is TooLargeText for a view that doesn't ask: for a file
// over the warning limit it adds that opening the file loads it anyway. ok
// is false for any other error, which this doesn't word.
func OpenToLoadText(name string, err error) (text string, ok bool) {
	var tl *core.TooLargeError
	if !errors.As(err, &tl) || !tl.Warn {
		return "", false
	}
	return config.WarnText(name, tl.Size, tl.Limit) + " " + config.OpenToLoadText, true
}

// AskLoad reports whether err says a file is over the limit above which it is
// read only once the user agrees, and returns the question to ask then.
func AskLoad(name string, err error) (question string, ok bool) {
	var tl *core.TooLargeError
	if !errors.As(err, &tl) || !tl.Warn {
		return "", false
	}
	return config.WarnText(name, tl.Size, tl.Limit) + " Load it anyway?", true
}
