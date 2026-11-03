package conv

import (
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// HostOptions returns the options every modal that hosts a conversation
// gives it: how the rows are drawn, the tree's keys and styles, who is
// reading, how the rows speak, and the size.
func HostOptions(o Options, keys treeview.KeyMap, dark bool, viewer string, voice ui.Voice, width, height int) []Option {
	return []Option{
		WithOptions(o),
		WithTreeKeys(keys),
		WithTreeStyles(treeview.DefaultStyles(dark)),
		WithViewer(viewer),
		WithVoice(voice),
		WithSize(width, height),
	}
}
