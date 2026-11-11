package releases

import (
	"charm.land/bubbles/v2/filepicker"
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
	// Download and DownloadAll save the file under the cursor, or every file
	// of the release. Select saves the file under the cursor in the Files pane.
	Download, DownloadAll, Select key.Binding

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

		Download:    modal.Binding("download", "download"),
		DownloadAll: modal.Binding("download_all", "download all"),
		Select:      modal.Binding("global.select", "download file"),
	}
	k.fillPanes(keys)
	k.Back.SetEnabled(false)
	return k
}

// pickKeys are the keys of the folder picker, from the same actions as the
// trees', so that remapping them remaps the picker. Esc and backspace are the
// app's: they reach the modal as Act.
type pickKeys struct {
	Up, Down, PageUp, PageDown, Top, Bottom key.Binding
	// Open goes into the folder under the cursor, and Back to the parent.
	Open, Back key.Binding
	// Select picks the folder the picker shows.
	Select key.Binding
	// Dismiss only shows in the help: the app routes it.
	Dismiss key.Binding
}

func newPickKeys(keys config.Keymap) pickKeys {
	modal := ui.In(keys, ctxModal)
	return pickKeys{
		Up:       modal.Binding("up", "up"),
		Down:     modal.Binding("down", "down"),
		PageUp:   modal.Binding("page_up", "page up"),
		PageDown: modal.Binding("page_down", "page down"),
		Top:      modal.Binding("top", "top"),
		Bottom:   modal.Binding("bottom", "bottom"),
		Open:     modal.Binding("expand", "open folder"),
		Back:     modal.Binding("collapse", "parent folder"),
		Select:   modal.Binding("global.select", "pick this folder"),
		Dismiss:  modal.Binding("global.dismiss", "cancel"),
	}
}

// filepicker is the picker's own map. Its Select is off: the modal picks the
// folder shown on the select key, since the picker only picks what is in it.
func (k pickKeys) filepicker() filepicker.KeyMap {
	return filepicker.KeyMap{
		GoToTop: k.Top, GoToLast: k.Bottom, Down: k.Down, Up: k.Up,
		PageUp: k.PageUp, PageDown: k.PageDown, Back: k.Back, Open: k.Open,
	}
}

// ShortHelp implements help.KeyMap.
func (k pickKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Open, k.Back, k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom, k.Dismiss}
}

// FullHelp implements help.KeyMap.
func (k pickKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// offFailed turns off the keys that do nothing while a read failed and
// nothing shows.
func (k *keyMap) offFailed() {
	for _, b := range []*key.Binding{
		&k.Open, &k.Download, &k.DownloadAll, &k.Select, &k.Next, &k.Prev, &k.Zoom, &k.jump, &k.Pane1, &k.Pane2,
		&k.DividerReset, &k.DividerUp, &k.DividerDown,
	} {
		b.SetEnabled(false)
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return append([]key.Binding{k.Dismiss, k.Back, k.Refresh, k.Open, k.Download, k.DownloadAll, k.Select}, k.paneBindings()...)
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
