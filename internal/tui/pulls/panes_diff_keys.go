package pulls

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// ctxDiff is the context of the keys of the diff of a pull request.
const ctxDiff = "pull_diff"

// diffKeys are the keys of the diff over the panes: those of the diff itself,
// and the one that switches between a file and the whole patch.
type diffKeys struct {
	diff diff.KeyMap
	// whole switches between the diff of the file and that of the whole patch.
	whole key.Binding
	// viewed marks the file viewed on GitHub, or takes the mark off.
	viewed key.Binding
	// readOn pages down, and marks the file viewed at its end and moves on to the
	// next one not viewed.
	readOn key.Binding
}

// newDiffKeys makes the keys of the diff from keys.
func newDiffKeys(keys config.Keymap) diffKeys {
	return diffKeys{
		diff:   diff.NewKeyMap(ui.Lookup(keys, ctxDiff)),
		whole:  ui.In(keys, ctxDiff).Binding("whole_patch", "whole patch"),
		viewed: ui.In(keys, ctxDiff).Binding("viewed", "mark viewed"),
		readOn: ui.In(keys, ctxDiff).Binding("read_on", "read on"),
	}
}
