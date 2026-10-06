package dashboard

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// KeyMap holds the keys of the dashboard.
type KeyMap struct {
	// Next and Prev move the focus through the panes, and Panes focus the
	// pane with that number.
	Next  key.Binding
	Prev  key.Binding
	Panes [numPanes]key.Binding
	// Zoom shows the focused pane alone, or every pane again, and Back
	// shows them again too.
	Zoom key.Binding
	Back key.Binding
	// Select opens what is under the cursor: a repository, a pull request
	// or issue, or what a notification is about.
	Select key.Binding
	// Open opens what is under the cursor in the browser.
	Open    key.Binding
	Refresh key.Binding
	// Filter and Sort open the filter of the repositories on its Filters
	// and Sort tabs, and ClearFilter clears it.
	Filter      key.Binding
	Sort        key.Binding
	ClearFilter key.Binding
	// NextOwner and PrevOwner switch the repositories between the viewer's
	// own and those of each organization, and NextList and PrevList the
	// work between its lists, with the keys of the tabs and each pane's own.
	NextOwner key.Binding
	PrevOwner key.Binding
	NextList  key.Binding
	PrevList  key.Binding
	// Here opens the repository of the current directory.
	Here key.Binding
	// Checks opens the pull request of the work under the cursor on its
	// checks.
	Checks key.Binding
	// Notifications names the key that shows every notification, which
	// the app handles.
	Notifications key.Binding
	// Up, Down, Left and Right move through the cards, the work and the
	// notifications.
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding

	// feed is the navigation of the repositories, which gets the keys
	// above only if the dashboard leaves them.
	feed feed.KeyMap
}

// The contexts of the keys of the dashboard: the screen, and a pane each.
const ctxDashboard = "dashboard"

// paneContext names the context of the keys of each pane.
var paneContext = [numPanes]string{"dashboard_pinned", "dashboard_repos", "dashboard_work", "dashboard_calendar", "dashboard_inbox"}

func newKeyMap(keys config.Keymap) KeyMap {
	screen, repos, work := ui.In(keys, ctxDashboard), ui.In(keys, paneContext[reposPane]), ui.In(keys, paneContext[workPane])
	k := KeyMap{
		Next:          screen.Binding("global.next_pane", "next pane"),
		Prev:          screen.Binding("global.prev_pane", "previous pane"),
		Zoom:          screen.Binding("global.zoom", "zoom"),
		Back:          screen.Binding("global.dismiss", "unzoom"),
		Select:        screen.Binding("global.select", "open"),
		Open:          screen.Binding("global.open", "browser"),
		Refresh:       screen.Binding("global.refresh", "refresh"),
		Filter:        repos.Binding("filter", "filter"),
		Sort:          repos.Binding("sort", "sort"),
		ClearFilter:   repos.Binding("clear_filter", "clear filters"),
		NextOwner:     repos.Either("next owner", "global.next_tab", "next_owner"),
		PrevOwner:     repos.Either("previous owner", "global.prev_tab", "prev_owner"),
		NextList:      work.Either("next list", "global.next_tab", "next_owner"),
		PrevList:      work.Either("previous list", "global.prev_tab", "prev_owner"),
		Here:          screen.Binding("current_repo", "this repo"),
		Checks:        work.Binding("checks", "checks"),
		Notifications: screen.Binding("global.notifications", "all notifications"),
		Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:          key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:         key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
	}
	actions := [numPanes]string{"global.pane_1", "global.pane_2", "global.pane_3", "global.pane_4", "global.pane_5"}
	for i, a := range actions {
		k.Panes[i] = screen.Binding(a, paneTitles[i])
	}
	k.Jump = ui.Jump(k.Panes[:]...)

	// The dashboard matches these keys first, so the list gets only the
	// keys it leaves it, such as f, which pages down there.
	f := feed.DefaultKeyMap()
	f.Retry = key.NewBinding(key.WithKeys(k.Refresh.Keys()...), key.WithHelp(k.Refresh.Help().Key, "retry"), key.WithDisabled())
	k.feed = f
	return k
}

// pane returns the pane that msg focuses, or -1.
func (k KeyMap) pane(msg tea.KeyPressMsg) paneID {
	for i, b := range k.Panes {
		if key.Matches(msg, b) {
			return paneID(i)
		}
	}
	return -1
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up, k.Down, k.Left, k.Right, k.Select, k.Checks, k.Filter, k.Sort, k.ClearFilter, k.NextOwner, k.NextList, k.Open,
		k.Next, k.Jump, k.Zoom, k.Back, k.Refresh, k.Here,
	}
}

// FullHelp implements help.KeyMap: the keys of the panes, which the
// dashboard matches first, and then its own.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down, k.Select, k.Open, k.Checks, k.NextOwner, k.PrevOwner, k.NextList, k.PrevList, k.ClearFilter, k.Filter, k.Sort, k.Notifications},
		{k.Next, k.Prev, k.Zoom, k.Back, k.Refresh, k.Here, k.Jump},
	}
}

// KeyLayers implements ui.Keyed: the keys of the dashboard, which work in
// every pane, and then those of the pane that has the focus: the list of
// repositories with its own keys and the list's, the calendar's, or the
// keys of the pinned cards, the work or the notifications.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys.state(s)
	screen := ui.ContextLayer(ctxDashboard,
		[]key.Binding{k.Next, k.Prev, k.Zoom, k.Back, k.Refresh, k.Here, k.Jump},
		[]key.Binding{k.Next, k.Jump, k.Zoom, k.Back, k.Refresh, k.Here})
	ctx := paneContext[s.focus]
	own := keyhelp.Layer{Bindings: k.paneKeys(s.focus), Short: k.paneShort(s.focus)}
	switch s.focus {
	case reposPane:
		return []keyhelp.Layer{screen, ui.MergeLayers(ctx, own, keyhelp.FromHelp("", s.repos.feedKeys(), false))}
	case calendarPane:
		return []keyhelp.Layer{screen, ui.ContextHelp(ctx, s.cal.KeyMap(), false)}
	default:
	}
	return []keyhelp.Layer{screen, ui.MergeLayers(ctx, own)}
}

// paneKeys returns the keys that work in the pane, in the order the
// dashboard matches them.
func (k KeyMap) paneKeys(p paneID) []key.Binding {
	switch p {
	case pinnedPane:
		return []key.Binding{k.Left, k.Right, k.Up, k.Down, k.Select, k.Open}
	case reposPane:
		return []key.Binding{k.Select, k.Open, k.NextOwner, k.PrevOwner, k.ClearFilter, k.Filter, k.Sort}
	case workPane:
		return []key.Binding{k.Up, k.Down, k.Select, k.Open, k.Checks, k.NextList, k.PrevList}
	case inboxPane:
		return []key.Binding{k.Up, k.Down, k.Select, k.Open}
	case calendarPane, numPanes:
	}
	return nil
}

// paneShort returns the keys of the pane worth a hint.
func (k KeyMap) paneShort(p paneID) []key.Binding {
	switch p {
	case pinnedPane:
		return []key.Binding{k.Up, k.Down, k.Left, k.Right, k.Select, k.Open}
	case reposPane:
		return []key.Binding{k.Select, k.Filter, k.Sort, k.ClearFilter, k.NextOwner, k.Open}
	case workPane:
		return []key.Binding{k.Up, k.Down, k.Select, k.Checks, k.NextList, k.Open}
	case inboxPane:
		return []key.Binding{k.Up, k.Down, k.Select, k.Open}
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
		k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.repos.filter().Active())
	case inboxPane:
		k.Select.SetHelp(k.Select.Help().Key, "open")
		if s.opener.MarksRead() {
			k.Select.SetHelp(k.Select.Help().Key, "open & read")
		}
	default:
	}
	k.Zoom.SetEnabled(k.Zoom.Enabled() && s.wide)
	k.Back.SetEnabled(k.Back.Enabled() && s.zoomed())
	k.Here.SetEnabled(k.Here.Enabled() && s.here != (core.RepoRef{}))
	return k
}
