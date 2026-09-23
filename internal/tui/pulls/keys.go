package pulls

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// keyMap holds the keys of the section, and those of its bubbles without
// the keys the section takes for itself.
type keyMap struct {
	Filter  key.Binding
	Refresh key.Binding
	Open    key.Binding

	feed feed.KeyMap
}

func newKeyMap(keys map[string][]string) keyMap {
	k := keyMap{
		Filter:  ui.Binding(keys, config.ActionFilter, "filter"),
		Refresh: ui.Binding(keys, config.ActionRefresh, "refresh"),
		Open:    ui.Binding(keys, config.ActionOpen, "open in browser"),
	}
	own := k.own()

	f := feed.DefaultKeyMap()
	f.Up, f.Down = without(f.Up, own), without(f.Down, own)
	f.PageUp, f.PageDown = without(f.PageUp, own), without(f.PageDown, own)
	f.Home, f.End = without(f.Home, own), without(f.End, own)
	// Refresh fetches failed chunks again too, so the feed's error row names
	// its keys.
	f.Retry = retry(k.Refresh)
	k.feed = f
	return k
}

// own returns the bindings the section handles before its bubbles.
func (k keyMap) own() []key.Binding {
	return []key.Binding{k.Filter, k.Refresh, k.Open}
}

// retry returns the refresh keys as a retry binding that starts disabled, so
// that a bubble enables it only while something failed.
func retry(refresh key.Binding) key.Binding {
	if !refresh.Enabled() {
		return key.NewBinding(key.WithDisabled())
	}
	return key.NewBinding(
		key.WithKeys(refresh.Keys()...),
		key.WithHelp(refresh.Help().Key, "retry"),
		key.WithDisabled(),
	)
}

// without returns b without the keys of taken, relabelled with what it has
// left, so that help never offers a key that does something else here.
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
	labels := make([]string, 0, 2)
	for _, k := range keys[:min(len(keys), 2)] {
		labels = append(labels, keyLabel(k))
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(labels, "/"), b.Help().Desc))
}

func keyLabel(k string) string {
	switch k {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "pgdown":
		return "pgdn"
	}
	return k
}

// keyHelp is a help.KeyMap made of fixed lists.
type keyHelp struct {
	short []key.Binding
	full  [][]key.Binding
}

func (h keyHelp) ShortHelp() []key.Binding  { return h.short }
func (h keyHelp) FullHelp() [][]key.Binding { return h.full }

// Help implements ui.Section. It lists the keys of the current view.
func (s *Section) Help() help.KeyMap {
	k, f := s.keys, s.keys.feed
	if !s.hasRepo {
		return keyHelp{}
	}
	return keyHelp{
		short: []key.Binding{f.Up, f.Down, k.Filter, k.Open},
		full: [][]key.Binding{
			{f.Up, f.Down, f.PageUp, f.PageDown},
			{f.Home, f.End},
			{k.Filter, k.Refresh, k.Open},
		},
	}
}
