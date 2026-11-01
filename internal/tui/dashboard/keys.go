package dashboard

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// KeyMap holds the keys of the dashboard.
type KeyMap struct {
	// Next and Prev move the focus through the panes, and Panes focus the
	// pane with that number.
	Next  key.Binding
	Prev  key.Binding
	Panes [numPanes]key.Binding
	// Zoom shows the focused pane alone, or every pane again.
	Zoom key.Binding
	// Select opens what is under the cursor: a repository, a pull request
	// or issue, or what a notification is about.
	Select  key.Binding
	Refresh key.Binding
	// Filter and Sort open the filter of the repositories on its Filters
	// and Sort tabs, and ClearFilter clears it.
	Filter      key.Binding
	Sort        key.Binding
	ClearFilter key.Binding
	// ToggleFilter switches the filter off, and back on.
	ToggleFilter key.Binding
	// GroupBy opens the choice of the levels the repositories are grouped
	// by, and ToggleGroups switches the grouping off, and back on.
	GroupBy      key.Binding
	ToggleGroups key.Binding
	// ToggleSort switches the sort off, and back on.
	ToggleSort key.Binding
	// NextOwner and PrevOwner switch the repositories between the viewer's
	// own and those of each organization, and NextList and PrevList the
	// work between its lists, with the keys of the tabs and each pane's own.
	NextOwner key.Binding
	PrevOwner key.Binding
	NextList  key.Binding
	PrevList  key.Binding
	// Checks opens the pull request of the work under the cursor on its
	// checks.
	Checks key.Binding
	// Up, Down, Left and Right move through the cards, WorkUp and WorkDown
	// through the work, and InboxUp and InboxDown through the
	// notifications, each with the keys of its pane.
	Up        key.Binding `keymap:"dashboard_pinned.up" help:"up"`
	Down      key.Binding `keymap:"dashboard_pinned.down" help:"down"`
	Left      key.Binding `keymap:"dashboard_pinned.left" help:"left"`
	Right     key.Binding `keymap:"dashboard_pinned.right" help:"right"`
	WorkUp    key.Binding `keymap:"dashboard_work.up" help:"up"`
	WorkDown  key.Binding `keymap:"dashboard_work.down" help:"down"`
	InboxUp   key.Binding `keymap:"dashboard_inbox.up" help:"up"`
	InboxDown key.Binding `keymap:"dashboard_inbox.down" help:"down"`
	InboxRead key.Binding `keymap:"dashboard_inbox.read" help:"read"`
	InboxDone key.Binding `keymap:"dashboard_inbox.done" help:"done"`

	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding

	// feed is the navigation of the repositories, which gets the keys
	// above only if the dashboard leaves them.
	feed feed.KeyMap
	// search are the keys of the prompt of the list's find and filter.
	search cmdline.KeyMap
	// order are the keys of the picker of the levels to group by.
	order orderpick.KeyMap
	// cal moves through the days of the contributions.
	cal calendar.KeyMap
	// confirm answers the question that marking a thread asks.
	confirm confirm.KeyMap
}

// The contexts of the keys of the dashboard: the screen, and a pane each.
const ctxDashboard = "dashboard"

// paneContext names the context of the keys of each pane.
var paneContext = [numPanes]string{"dashboard_pinned", "dashboard_repos", "dashboard_work", "dashboard_calendar", "dashboard_inbox"}

func newKeyMap(keys config.Keymap) KeyMap {
	screen, repos, work := ui.In(keys, ctxDashboard), ui.In(keys, paneContext[reposPane]), ui.In(keys, paneContext[workPane])
	k := KeyMap{
		Next:         screen.Binding("global.next_pane", "next pane"),
		Prev:         screen.Binding("global.prev_pane", "previous pane"),
		Zoom:         screen.Binding("global.zoom", "zoom"),
		Select:       screen.Binding("global.select", "open"),
		Refresh:      screen.Binding("global.refresh", "refresh"),
		Filter:       repos.Binding("filter", "filter"),
		Sort:         repos.Binding("sort", "sort"),
		ClearFilter:  repos.Binding("clear_filter", "clear filters"),
		ToggleFilter: repos.Binding("filter_toggle", "filter on/off"),
		GroupBy:      repos.Binding("group_by", "group by"),
		ToggleGroups: repos.Binding("group_toggle", "group on/off"),
		ToggleSort:   repos.Binding("sort_toggle", "sort on/off"),
		NextOwner:    repos.Binding("global.next_tab", "next owner"),
		PrevOwner:    repos.Binding("global.prev_tab", "previous owner"),
		NextList:     work.Binding("global.next_tab", "next list"),
		PrevList:     work.Binding("global.prev_tab", "previous list"),
		Checks:       work.Binding("checks", "checks"),
	}
	keymap.Fill(&k, screen)
	actions := [numPanes]string{"global.pane_1", "global.pane_2", "global.pane_3", "global.pane_4", "global.pane_5"}
	for i, a := range actions {
		k.Panes[i] = screen.Binding(a, paneTitles[i])
	}
	k.Jump = ui.Jump(k.Panes[:]...)

	// The dashboard matches these keys first, so the list gets only the
	// keys it leaves it.
	k.feed = feed.NewKeyMap(repos)
	k.search = ui.SearchPromptKeys(keys)
	k.order = ui.OrderPickKeys(keys)
	k.cal = calendar.NewKeyMap(ui.In(keys, paneContext[calendarPane]))
	k.confirm = confirm.NewKeyMap(ui.Lookup(keys, "confirm"))
	return k
}

// pane returns the pane that msg focuses, or -1.
func (k KeyMap) pane(msg tea.KeyPressMsg) paneID {
	for i, b := range k.Panes {
		if keymap.Matches(msg, b) {
			return paneID(i)
		}
	}
	return -1
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Left, k.Right, k.Select, k.Checks, k.Filter, k.Sort, k.ClearFilter, k.NextOwner, k.NextList,
		k.Next, k.Jump, k.Zoom, k.Refresh,
	}
}

// FullHelp implements help.KeyMap: the keys of the panes, which the
// dashboard matches first, and then its own.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down, k.Select, k.Checks, k.NextOwner, k.PrevOwner, k.NextList, k.PrevList, k.ClearFilter, k.ToggleFilter, k.Filter, k.Sort, k.ToggleSort, k.GroupBy, k.ToggleGroups},
		{k.WorkUp, k.WorkDown, k.InboxUp, k.InboxDown, k.InboxRead, k.InboxDone},
		{k.Next, k.Prev, k.Zoom, k.Refresh, k.Jump},
	}
}

// KeyLayers implements ui.Keyed: the keys of the dashboard, which work in
// every pane, and then those of the pane that has the focus: the list of
// repositories with its own keys and the list's, the calendar's, or the
// keys of the pinned cards, the work or the notifications.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys.state(s)
	screen := ui.ContextLayer(ctxDashboard,
		[]key.Binding{k.Next, k.Prev, k.Zoom, k.Refresh, k.Jump},
		[]key.Binding{k.Next, k.Jump, k.Zoom, k.Refresh})
	ctx := paneContext[s.focus]
	own := keyhelp.Layer{Bindings: k.paneKeys(s.focus), Short: k.paneShort(s.focus)}
	switch s.focus {
	case reposPane:
		list, prompting := ui.FeedLayer(s.repos.current().Feed)
		if prompting {
			return []keyhelp.Layer{list}
		}
		return []keyhelp.Layer{screen, ui.MergeLayers(ctx, own, list)}
	case calendarPane:
		return []keyhelp.Layer{screen, ui.ContextHelp(ctx, s.cal.KeyMap(), false)}
	default:
	}
	return []keyhelp.Layer{screen, ui.MergeLayers(ctx, own)}
}

// Capturing implements ui.Capturer: the list of repositories takes every
// key while its find or filter prompt is open.
func (s *Section) Capturing() bool {
	return s.focus == reposPane && s.repos.current().Feed.Capturing()
}

// paneKeys returns the keys that work in the pane, in the order the
// dashboard matches them.
func (k KeyMap) paneKeys(p paneID) []key.Binding {
	switch p {
	case pinnedPane:
		return []key.Binding{k.Left, k.Right, k.Up, k.Down, k.Select}
	case reposPane:
		return []key.Binding{k.Select, k.NextOwner, k.PrevOwner, k.ClearFilter, k.ToggleFilter, k.Filter, k.Sort, k.ToggleSort, k.GroupBy, k.ToggleGroups}
	case workPane:
		return []key.Binding{k.WorkUp, k.WorkDown, k.Select, k.Checks, k.NextList, k.PrevList}
	case inboxPane:
		return []key.Binding{k.InboxUp, k.InboxDown, k.Select, k.InboxRead, k.InboxDone}
	case calendarPane, numPanes:
	}
	return nil
}

// paneShort returns the keys of the pane worth a hint.
func (k KeyMap) paneShort(p paneID) []key.Binding {
	switch p {
	case pinnedPane:
		return []key.Binding{k.Up, k.Down, k.Left, k.Right, k.Select}
	case reposPane:
		return []key.Binding{k.Select, k.Filter, k.Sort, k.ClearFilter, k.NextOwner}
	case workPane:
		return []key.Binding{k.WorkUp, k.WorkDown, k.Select, k.Checks, k.NextList}
	case inboxPane:
		return []key.Binding{k.InboxUp, k.InboxDown, k.Select, k.InboxRead, k.InboxDone}
	case calendarPane, numPanes:
	}
	return nil
}

// state returns k as the dashboard takes it with the focus on its pane:
// zoom while every pane fits, and the way back while one is zoomed, and
// the way each pane names what its keys do.
func (k KeyMap) state(s *Section) KeyMap {
	switch s.focus {
	case reposPane:
		active := s.repos.filter().Active() || s.repos.toggle.Off()
		k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && active)
		k.ToggleFilter.SetEnabled(k.ToggleFilter.Enabled() && active)
		k.ToggleGroups.SetEnabled(k.ToggleGroups.Enabled() && len(s.repos.grouping.Levels) > 0)
	case inboxPane:
		k.Select.SetHelp(k.Select.Help().Key, "open")
		if s.opener.MarksRead() {
			k.Select.SetHelp(k.Select.Help().Key, "open & read")
		}
	default:
	}
	k.Zoom.SetEnabled(k.Zoom.Enabled() && s.zoom.CanZoom())
	if s.zoomed() {
		k.Zoom.SetHelp(k.Zoom.Help().Key, "unzoom")
	}
	return k
}
