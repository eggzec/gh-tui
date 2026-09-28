package dashboard

import (
	"slices"
	"strings"

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
	// Filter and Sort name the keys that open the filter of the
	// repositories on its Filters and Sort tabs, which the app handles,
	// and ClearFilter clears it.
	Filter      key.Binding
	Sort        key.Binding
	ClearFilter key.Binding
	// NextOwner and PrevOwner switch the repositories between the viewer's
	// own and those of each organization, and the work between its lists.
	NextOwner key.Binding
	PrevOwner key.Binding
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

	// feed is the navigation of the repositories, without the keys above.
	feed feed.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		Next:          ui.Binding(keys, config.ActionNextTab, "next pane"),
		Prev:          ui.Binding(keys, config.ActionPrevTab, "previous pane"),
		Zoom:          ui.Binding(keys, config.ActionZoom, "zoom"),
		Back:          ui.Binding(keys, config.ActionBack, "unzoom"),
		Select:        ui.Binding(keys, config.ActionSelect, "open"),
		Open:          ui.Binding(keys, config.ActionOpen, "browser"),
		Refresh:       ui.Binding(keys, config.ActionRefresh, "refresh"),
		Filter:        ui.Binding(keys, config.ActionFilter, "filter"),
		Sort:          ui.Binding(keys, config.ActionSort, "sort"),
		ClearFilter:   ui.Binding(keys, config.ActionClearFilter, "clear filters"),
		NextOwner:     ui.Binding(keys, config.ActionNextOwner, "next owner"),
		PrevOwner:     ui.Binding(keys, config.ActionPrevOwner, "previous owner"),
		Here:          ui.Binding(keys, config.ActionCurrentRepo, "this repo"),
		Checks:        ui.Binding(keys, config.ActionChecks, "checks"),
		Notifications: ui.Binding(keys, config.ActionNotifications, "all notifications"),
		Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:          key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:         key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
	}
	actions := [numPanes]string{config.ActionPane1, config.ActionPane2, config.ActionPane3, config.ActionPane4, config.ActionPane5}
	labels := make([]string, 0, numPanes)
	for i, a := range actions {
		k.Panes[i] = ui.Binding(keys, a, paneTitles[i])
		if k.Panes[i].Enabled() {
			labels = append(labels, k.Panes[i].Help().Key)
		}
	}
	k.Jump = key.NewBinding(key.WithDisabled())
	if len(labels) > 0 {
		k.Jump = key.NewBinding(key.WithKeys(labels...), key.WithHelp(labels[0]+"-"+labels[len(labels)-1], "focus pane"))
	}

	// PR5: the dashboard, and the app for the filter, match these keys
	// first, so dropping them from the list only keeps the collisions out
	// of help.
	own := []key.Binding{k.Select, k.Open, k.Refresh, k.Filter, k.Sort, k.ClearFilter, k.NextOwner, k.PrevOwner, k.Here, k.Next, k.Prev, k.Zoom, k.Back}
	f := feed.DefaultKeyMap()
	f.Up = free(f.Up, own)
	f.Down = free(f.Down, own)
	f.PageUp = free(f.PageUp, own)
	f.PageDown = free(f.PageDown, own)
	f.Home = free(f.Home, own)
	f.End = free(f.End, own)
	f.Retry = key.NewBinding(key.WithKeys(k.Refresh.Keys()...), key.WithHelp(k.Refresh.Help().Key, "retry"), key.WithDisabled())
	k.feed = f
	return k
}

// free drops the keys of b that the dashboard binds itself, such as "f",
// which the list uses for page down and the dashboard for the filter.
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
		k.Up, k.Down, k.Left, k.Right, k.Select, k.Checks, k.Filter, k.Sort, k.ClearFilter, k.NextOwner, k.Open,
		k.Next, k.Jump, k.Zoom, k.Back, k.Refresh, k.Here,
	}
}

// FullHelp implements help.KeyMap: the keys of the panes, which the
// dashboard matches first, and then its own.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down, k.Select, k.Open, k.Checks, k.NextOwner, k.PrevOwner, k.ClearFilter, k.Filter, k.Sort, k.Notifications},
		{k.Next, k.Prev, k.Zoom, k.Back, k.Refresh, k.Here, k.Jump},
	}
}

// KeyLayers implements ui.Keyed: the keys of the focused pane and of the
// dashboard, named for what they do there, and then those of the list of
// repositories or of the calendar, whichever has the focus.
func (s *Section) KeyLayers() []keyhelp.Layer {
	own := keyhelp.FromHelp("dashboard", s.keys.state(s), false)
	switch s.focus {
	case reposPane:
		return []keyhelp.Layer{own, keyhelp.FromHelp("list", s.repos.feedKeys(), false)}
	case calendarPane:
		return []keyhelp.Layer{own, keyhelp.FromHelp("calendar", s.cal.KeyMap(), false)}
	default:
	}
	return []keyhelp.Layer{own}
}

// state returns k as the dashboard takes it with the focus on its pane:
// the keys of that pane, named for what they do there, zoom while every
// pane fits, and the way back while one is zoomed.
func (k KeyMap) state(s *Section) KeyMap {
	panes := map[paneID][]*key.Binding{
		pinnedPane: {&k.Left, &k.Right, &k.Up, &k.Down, &k.Select, &k.Open},
		reposPane:  {&k.NextOwner, &k.PrevOwner, &k.ClearFilter, &k.Select, &k.Open, &k.Filter, &k.Sort},
		workPane:   {&k.NextOwner, &k.PrevOwner, &k.Up, &k.Down, &k.Select, &k.Checks, &k.Open},
		inboxPane:  {&k.Up, &k.Down, &k.Select, &k.Open},
	}
	for _, b := range []*key.Binding{
		&k.Left, &k.Right, &k.Up, &k.Down, &k.Select, &k.Open, &k.Checks,
		&k.NextOwner, &k.PrevOwner, &k.ClearFilter, &k.Filter, &k.Sort, &k.Notifications,
	} {
		b.SetEnabled(b.Enabled() && slices.Contains(panes[s.focus], b))
	}
	switch s.focus {
	case reposPane:
		k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.repos.filter().active())
	case workPane:
		k.NextOwner.SetHelp(k.NextOwner.Help().Key, "next list")
		k.PrevOwner.SetHelp(k.PrevOwner.Help().Key, "previous list")
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
