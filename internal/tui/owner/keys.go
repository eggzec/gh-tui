package owner

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// KeyMap holds the keys of the page.
type KeyMap struct {
	// Next and Prev move the focus through the panes, and Panes focus the
	// pane with that number.
	Next  key.Binding
	Prev  key.Binding
	Panes [numPanes]key.Binding
	// NextTab and PrevTab switch between the tabs of the list pane.
	NextTab key.Binding
	PrevTab key.Binding
	// Zoom shows the focused pane alone, or every pane again.
	Zoom key.Binding
	// Back shows every pane again while one is zoomed, or else the page
	// this one was opened from, or else the screen before the page.
	Back key.Binding
	// Select opens what is under the cursor: a repository on the
	// repository screen, a person or an organization on their page, and a
	// team, which has no page here, in the browser.
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
	// Up, Down, Left and Right move through the cards.
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding

	// feed is the navigation of the lists, which gets the keys above only
	// if the page leaves them.
	feed feed.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		Next:        ui.Binding(keys, config.ActionNextTab, "next pane"),
		Prev:        ui.Binding(keys, config.ActionPrevTab, "previous pane"),
		NextTab:     ui.Binding(keys, config.ActionNextFilter, "next tab"),
		PrevTab:     ui.Binding(keys, config.ActionPrevFilter, "previous tab"),
		Zoom:        ui.Binding(keys, config.ActionZoom, "zoom"),
		Back:        ui.Binding(keys, config.ActionBack, "back"),
		Select:      ui.Binding(keys, config.ActionSelect, "open"),
		Open:        ui.Binding(keys, config.ActionOpen, "browser"),
		Refresh:     ui.Binding(keys, config.ActionRefresh, "refresh"),
		Filter:      ui.Binding(keys, config.ActionFilter, "filter"),
		Sort:        ui.Binding(keys, config.ActionSort, "sort"),
		ClearFilter: ui.Binding(keys, config.ActionClearFilter, "clear filters"),
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:        key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:       key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
	}
	actions := [numPanes]string{config.ActionPane1, config.ActionPane2, config.ActionPane3, config.ActionPane4}
	names := [numPanes]string{paneTitles[pinnedPane], "List", paneTitles[readmePane], paneTitles[calendarPane]}
	for i, a := range actions {
		k.Panes[i] = ui.Binding(keys, a, names[i])
	}
	k.Jump = ui.Jump(k.Panes[:]...)

	// The page, and the app for the filter, match these keys first, so
	// the list gets only the keys they leave it, such as g, which goes
	// to the first row there.
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
		k.Up, k.Down, k.Left, k.Right, k.Select, k.Filter, k.Sort, k.ClearFilter, k.Open,
		k.NextTab, k.Next, k.Jump, k.Zoom, k.Back, k.Refresh,
	}
}

// FullHelp implements help.KeyMap: the keys of the panes, which the page
// matches first, and then its own.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down, k.Select, k.Open, k.NextTab, k.PrevTab, k.ClearFilter, k.Filter, k.Sort},
		{k.Next, k.Prev, k.Zoom, k.Back, k.Refresh, k.Jump},
	}
}

// KeyLayers implements ui.Keyed: the keys of the focused pane and of the
// page, named for what they do there, and then those of the list on view
// while it has the focus.
func (s *Section) KeyLayers() []keyhelp.Layer {
	own := keyhelp.FromHelp("profile", s.keys.state(s), false)
	if l := s.page.list(); l != nil && s.page.focus == listPane {
		return []keyhelp.Layer{own, keyhelp.FromHelp("list", l.feed().KeyMap(), false)}
	}
	if side, ok := s.sideLayer(); ok {
		return []keyhelp.Layer{own, side}
	}
	return []keyhelp.Layer{own}
}

// state returns k as the page takes it with the focus on its pane: the
// keys of that pane, zoom while every pane fits, and the way back while
// there is one.
func (k KeyMap) state(s *Section) KeyMap {
	focus := pinnedPane
	if s.page != nil {
		focus = s.page.focus
	}
	panes := map[paneID][]*key.Binding{
		pinnedPane: {&k.Left, &k.Right, &k.Up, &k.Down, &k.Select, &k.Open},
		listPane:   {&k.NextTab, &k.PrevTab, &k.ClearFilter, &k.Select, &k.Open, &k.Filter, &k.Sort},
		readmePane: {&k.Open},
	}
	for _, b := range []*key.Binding{&k.Left, &k.Right, &k.Up, &k.Down, &k.Select, &k.Open, &k.NextTab, &k.PrevTab, &k.ClearFilter, &k.Filter, &k.Sort} {
		b.SetEnabled(b.Enabled() && s.page != nil && slices.Contains(panes[focus], b))
	}
	if focus == listPane {
		l := s.repoTab()
		k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && l != nil && l.Filter().Active())
		k.Filter.SetEnabled(k.Filter.Enabled() && l != nil)
		k.Sort.SetEnabled(k.Sort.Enabled() && l != nil)
		// The tabs are known once the header says whose the page is.
		tabs := s.page != nil && s.page.header.ok
		k.NextTab.SetEnabled(k.NextTab.Enabled() && tabs)
		k.PrevTab.SetEnabled(k.PrevTab.Enabled() && tabs)
		if l := s.page.list(); l != nil {
			// A team has no page here, so only the open key opens one;
			// with nothing under the cursor, that opens the tab on GitHub.
			_, team := l.(*teamList)
			_, ok := l.selection(s)
			k.Select.SetEnabled(k.Select.Enabled() && ok && !team)
		}
	}
	// An organization has no calendar, so its pane has no key to name.
	if !s.hasPane(calendarPane) && k.Panes[calendarPane].Enabled() {
		k.Panes[calendarPane].SetEnabled(false)
		k.Jump = ui.Jump(k.Panes[:]...)
	}
	k.Zoom.SetEnabled(k.Zoom.Enabled() && s.wide)
	k.Refresh.SetEnabled(k.Refresh.Enabled() && s.page != nil)
	switch {
	case s.zoomed():
		k.Back.SetHelp(k.Back.Help().Key, "unzoom")
	case len(s.back) > 0:
		k.Back.SetHelp(k.Back.Help().Key, "previous page")
	}
	return k
}
