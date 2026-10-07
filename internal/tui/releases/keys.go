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

// ctxModal is the context of the keys of the modal, which has one pane.
const ctxModal = "release_modal"

func newKeyMap(keys config.Keymap) keyMap {
	modal := ui.In(keys, ctxModal)
	k := keyMap{
		Back:    modal.Binding("global.dismiss", "back"),
		Open:    modal.Binding("global.open", "open in browser"),
		Refresh: modal.Binding("global.refresh", "retry"),
	}
	// The modal matches its own keys first, so the thread gets only the
	// keys it leaves it.
	t := thread.NewKeyMap(modal.Of)
	// The files come with the release, so there is nothing for the
	// thread to retry.
	t.Retry = key.NewBinding(key.WithHelp("", t.Retry.Help().Desc), key.WithDisabled())
	k.thread = t
	return k
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding { return []key.Binding{k.Back, k.Open, k.Refresh} }

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
