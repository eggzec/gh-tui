package releases

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// keyMap holds the keys of the modal. The trees get only the keys it leaves
// them.
type keyMap struct {
	// Dismiss closes the modal. Back is off: the modal has no step before
	// it, and takes the key so that it does nothing.
	Dismiss, Back key.Binding
	// Refresh reads the release again after a read failed.
	Refresh key.Binding
	// Open shows the release in the browser.
	Open key.Binding

	// The pane keys sit in the map itself, where the checks see them.
	Next, Prev, Zoom       key.Binding
	jump                   key.Binding
	Pane1, Pane2           key.Binding
	DividerReset           key.Binding
	DividerUp, DividerDown key.Binding
}

// ctxModal is the context of the keys of the modal.
const ctxModal = "release_modal"

func newKeyMap(keys config.Keymap) keyMap {
	modal := ui.In(keys, ctxModal)
	k := keyMap{
		Dismiss: modal.Binding("global.dismiss", "close"),
		Back:    modal.Binding("global.back", "back"),
		Refresh: modal.Binding("global.refresh", "retry"),
		Open:    modal.Binding("open", "open on GitHub"),
	}
	k.fillPanes(keys)
	k.Back.SetEnabled(false)
	return k
}

// offFailed turns off the keys that do nothing while a read failed and
// nothing shows.
func (k *keyMap) offFailed() {
	for _, b := range []*key.Binding{
		&k.Open, &k.Next, &k.Prev, &k.Zoom, &k.jump, &k.Pane1, &k.Pane2,
		&k.DividerReset, &k.DividerUp, &k.DividerDown,
	} {
		b.SetEnabled(false)
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return append([]key.Binding{k.Dismiss, k.Back, k.Refresh, k.Open}, k.paneBindings()...)
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
