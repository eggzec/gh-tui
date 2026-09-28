package releases

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
)

// keyMap holds the keys of the modal and of its thread, which gets only
// the keys the modal leaves it.
type keyMap struct {
	Back key.Binding
	// Open shows the release on GitHub.
	Open key.Binding
	// Refresh reads the release again after a read failed.
	Refresh key.Binding

	thread thread.KeyMap
}

func newKeyMap(keys map[string][]string) keyMap {
	k := keyMap{
		Back:    ui.Binding(keys, config.ActionBack, "back"),
		Open:    ui.Binding(keys, config.ActionOpen, "open in browser"),
		Refresh: ui.Binding(keys, config.ActionRefresh, "retry"),
	}
	// The modal matches its own keys first, so the thread gets only the
	// keys it leaves it.
	t := thread.DefaultKeyMap()
	t.Toggle = ui.Binding(keys, config.ActionSelect, t.Toggle.Help().Desc)
	// The files come with the release, so there is nothing for the
	// thread to retry.
	t.Retry = key.NewBinding(key.WithDisabled())
	k.thread = t
	return k
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding { return []key.Binding{k.Back, k.Open, k.Refresh} }

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
