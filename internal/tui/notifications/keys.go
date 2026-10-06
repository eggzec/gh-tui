package notifications

import (
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
	// Filter names the key that opens the filter, which the app handles,
	// and ClearFilter goes back to the unread threads.
	Filter      key.Binding
	ClearFilter key.Binding
	Refresh     key.Binding

	// feed is the navigation of the list, which gets the keys above
	// only if the section leaves them.
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
		ClearFilter: ui.Binding(keys, config.ActionClearFilter, "clear filters"),
		Refresh:     ui.Binding(keys, config.ActionRefresh, "refresh"),
	}
	// The section, and the app for the filter, match these keys first,
	// so the list gets only the keys they leave it, such as g, which
	// goes to the first row there.
	f := feed.DefaultKeyMap()
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
	return []key.Binding{k.Select, k.Open, k.MarkRead, k.MarkDone, k.MarkAllRead, k.Filter, k.ClearFilter, k.Refresh}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.MarkRead, k.MarkDone, k.Filter, k.ClearFilter}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.bindings()} }
