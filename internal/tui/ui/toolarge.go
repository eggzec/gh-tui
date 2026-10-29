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
	return config.TooLargeText(name, tl.Size, tl.Limit)
}
