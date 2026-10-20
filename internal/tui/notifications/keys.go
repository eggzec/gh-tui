package notifications

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// KeyMap holds the keys of the section. It implements help.KeyMap.
type KeyMap struct {
	Select   key.Binding
	Open     key.Binding
	MarkRead key.Binding
	MarkDone key.Binding
	// MarkAllRead is for help, which lists the read all command by it.
	MarkAllRead key.Binding
	// Filter opens the filter, and ClearFilter goes back to the unread
	// threads.
	Filter      key.Binding
	ClearFilter key.Binding
	// ToggleFilter switches the filter off, and back on.
	ToggleFilter key.Binding
	// GroupBy opens the choice of the levels the list is grouped by, and
	// ToggleGroups switches the grouping off, and back on.
	GroupBy      key.Binding
	ToggleGroups key.Binding
	Refresh      key.Binding

	// order are the keys of the picker of the levels.
	order orderpick.KeyMap
	// confirm answers the question that marking all read asks.
	confirm ui.ConfirmKeys
	// feed is the navigation of the list, which gets the keys above
	// only if the section leaves them.
	feed feed.KeyMap
	// mark is the key that marks rows of the list.
	mark feed.MarkKeys
	// search are the keys of the prompt of the list's find and filter.
	search cmdline.KeyMap
}

// ctxScreen is the context of the keys of the notifications screen.
const ctxScreen = "notifications"

func newKeyMap(keys config.Keymap) KeyMap {
	screen := ui.In(keys, ctxScreen)
	k := KeyMap{
		Select:   screen.Binding("global.select", "open & read"),
		Open:     screen.Binding("global.open", "open"),
		MarkRead: screen.Binding("read", "read"),
		MarkDone: screen.Binding("done", "done"),
		// Reading everything is the read command with all, which has no
		// key of its own: help lists it by the read action it is part of.
		MarkAllRead:  keymap.Record(keymap.Unbound(key.NewBinding(key.WithHelp("", "read all"))), "notifications.read"),
		Filter:       screen.Binding("filter", "filter"),
		ClearFilter:  screen.Binding("clear_filter", "clear filters"),
		ToggleFilter: screen.Binding("filter_toggle", "filter on/off"),
		GroupBy:      screen.Binding("group_by", "group by"),
		ToggleGroups: screen.Binding("group_toggle", "group on/off"),
		Refresh:      screen.Binding("global.refresh", "refresh"),
		order:        ui.OrderPickKeys(keys),
		confirm:      ui.NewConfirmKeys(keys),
		// The section matches these keys first, so the list gets only the
		// keys it leaves it. Refresh retries what failed, so the list's error
		// row names its keys.
		feed:   feed.NewKeyMap(screen),
		mark:   feed.NewMarkKeys(screen),
		search: ui.SearchPromptKeys(keys),
	}
	return k
}

func (k KeyMap) bindings() []key.Binding {
	return []key.Binding{k.Select, k.Open, k.MarkRead, k.MarkDone, k.MarkAllRead, k.Filter, k.ClearFilter, k.ToggleFilter, k.GroupBy, k.ToggleGroups, k.Refresh}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.MarkRead, k.MarkDone, k.Filter, k.ClearFilter}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.bindings()} }
