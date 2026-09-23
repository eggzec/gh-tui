package notifications

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// KeyMap holds the keys of the section. It implements help.KeyMap.
type KeyMap struct {
	Select      key.Binding
	Open        key.Binding
	MarkRead    key.Binding
	MarkDone    key.Binding
	MarkAllRead key.Binding
	Filter      key.Binding
	Refresh     key.Binding

	// feed is the navigation of the list, without the keys above.
	feed feed.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		Select:      ui.Binding(keys, config.ActionSelect, "open & read"),
		Open:        ui.Binding(keys, config.ActionOpen, "open"),
		MarkRead:    ui.Binding(keys, config.ActionMarkRead, "read"),
		MarkDone:    ui.Binding(keys, config.ActionMarkDone, "done"),
		MarkAllRead: ui.Binding(keys, config.ActionMarkAllRead, "all read"),
		Filter:      ui.Binding(keys, config.ActionFilter, "filter"),
		Refresh:     ui.Binding(keys, config.ActionRefresh, "refresh"),
	}
	own := k.bindings()
	f := feed.DefaultKeyMap()
	f.Up = free(f.Up, own)
	f.Down = free(f.Down, own)
	f.PageUp = free(f.PageUp, own)
	f.PageDown = free(f.PageDown, own)
	f.Home = free(f.Home, own)
	f.End = free(f.End, own)
	// The section handles refresh before the list, and a refresh retries
	// what failed, so the list's error row names the refresh keys.
	f.Retry = key.NewBinding(
		key.WithKeys(k.Refresh.Keys()...),
		key.WithHelp(k.Refresh.Help().Key, "retry"),
		key.WithDisabled(),
	)
	k.feed = f
	return k
}

func (k KeyMap) bindings() []key.Binding {
	return []key.Binding{k.Select, k.Open, k.MarkRead, k.MarkDone, k.MarkAllRead, k.Filter, k.Refresh}
}

// free drops the keys of b that the section binds itself, such as "f",
// which the list uses for page down and the section for the filter.
func free(b key.Binding, taken []key.Binding) key.Binding {
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

// withFeed returns k with the list's current keys, whose retry the list
// enables only while a fetch has failed.
func (k KeyMap) withFeed(f feed.KeyMap) KeyMap {
	k.feed = f
	return k
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.feed.Up, k.feed.Down, k.Select, k.MarkRead, k.MarkDone, k.Filter, k.feed.Retry}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return append(k.feed.FullHelp(),
		[]key.Binding{k.Select, k.Open, k.Filter, k.Refresh},
		[]key.Binding{k.MarkRead, k.MarkDone, k.MarkAllRead},
	)
}
