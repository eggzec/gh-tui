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
	// Filter opens the filter, and ClearFilter goes back to the unread
	// threads.
	Filter      key.Binding
	ClearFilter key.Binding
	Refresh     key.Binding

	// feed is the navigation of the list, which gets the keys above
	// only if the section leaves them.
	feed feed.KeyMap
}

// ctxScreen is the context of the keys of the notifications screen.
const ctxScreen = "notifications"

func newKeyMap(keys config.Keymap) KeyMap {
	screen := ui.In(keys, ctxScreen)
	k := KeyMap{
		Select:      screen.Binding("global.select", "open & read"),
		Open:        screen.Binding("global.open", "open"),
		MarkRead:    screen.Binding("read", "read"),
		MarkDone:    screen.Binding("done", "done"),
		MarkAllRead: screen.Binding("read_all", "all read"),
		Filter:      screen.Binding("filter", "filter"),
		ClearFilter: screen.Binding("clear_filter", "clear filters"),
		Refresh:     screen.Binding("global.refresh", "refresh"),
	}
	// The section matches these keys first, so the list gets only the
	// keys it leaves it, such as f, which pages down there.
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
