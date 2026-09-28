package releases

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
)

// keyMap holds the keys of the modal, and those of its thread without the
// keys the modal takes for itself.
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
	// PR5: the modal matches its own keys first, so dropping them from
	// the thread only keeps the collisions out of help.
	own := []key.Binding{k.Back, k.Open, k.Refresh}
	t := thread.DefaultKeyMap()
	t.Up, t.Down = without(t.Up, own), without(t.Down, own)
	t.PageUp, t.PageDown = without(t.PageUp, own), without(t.PageDown, own)
	t.HalfPageUp, t.HalfPageDown = without(t.HalfPageUp, own), without(t.HalfPageDown, own)
	t.Top, t.Bottom = without(t.Top, own), without(t.Bottom, own)
	t.Toggle = without(ui.Binding(keys, config.ActionSelect, t.Toggle.Help().Desc), own)
	// The files come with the release, so there is nothing for the
	// thread to retry.
	t.Retry = key.NewBinding(key.WithDisabled())
	k.thread = t
	return k
}

// without drops the keys of b that the modal binds itself.
func without(b key.Binding, taken []key.Binding) key.Binding {
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool {
		return slices.ContainsFunc(taken, func(t key.Binding) bool {
			return t.Enabled() && slices.Contains(t.Keys(), k)
		})
	})
	if len(keys) == len(b.Keys()) {
		return b
	}
	if len(keys) == 0 {
		return key.NewBinding(key.WithDisabled())
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(keys, "/"), b.Help().Desc))
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding { return []key.Binding{k.Back, k.Open, k.Refresh} }

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
