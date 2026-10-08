package owner

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
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
	// Back shows every pane again while one is zoomed.
	Back key.Binding
	// Select opens what is under the cursor: a repository on the
	// repository screen, a person or an organization on their page, and a
	// team, which has no page here, in the browser.
	Select key.Binding
	// Open opens what is under the cursor in the browser.
	Open    key.Binding
	Refresh key.Binding
	// Filter and Sort open the filter of the repositories on its Filters
	// and Sort tabs, and ClearFilter clears it.
	Filter      key.Binding
	Sort        key.Binding
	ClearFilter key.Binding
	// Up, Down, Left and Right move through the cards.
	Up    key.Binding `keymap:"owner_pinned.up" help:"up"`
	Down  key.Binding `keymap:"owner_pinned.down" help:"down"`
	Left  key.Binding `keymap:"owner_pinned.left" help:"left"`
	Right key.Binding `keymap:"owner_pinned.right" help:"right"`

	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding

	// feed is the navigation of the lists, which gets the keys above only
	// if the page leaves them.
	feed feed.KeyMap
	// cal moves through the days of a user's contributions.
	cal calendar.KeyMap
}

// ctxPage is the context of the keys of the page, which work in every
// pane.
const ctxPage = "owner"

// paneContext names the context of the keys of each pane.
var paneContext = [numPanes]string{"owner_pinned", "owner_list", "owner_readme", "owner_calendar"}

func newKeyMap(keys config.Keymap) KeyMap {
	page, list := ui.In(keys, ctxPage), ui.In(keys, paneContext[listPane])
	k := KeyMap{
		Next:        page.Binding("global.next_pane", "next pane"),
		Prev:        page.Binding("global.prev_pane", "previous pane"),
		NextTab:     list.Binding("global.next_tab", "next tab"),
		PrevTab:     list.Binding("global.prev_tab", "previous tab"),
		Zoom:        page.Binding("global.zoom", "zoom"),
		Back:        page.Binding("global.dismiss", "back"),
		Select:      page.Binding("global.select", "open"),
		Open:        page.Binding("global.open", "browser"),
		Refresh:     page.Binding("global.refresh", "refresh"),
		Filter:      list.Binding("filter", "filter"),
		Sort:        list.Binding("sort", "sort"),
		ClearFilter: list.Binding("clear_filter", "clear filters"),
	}
	keymap.Fill(&k, page)
	actions := [numPanes]string{"global.pane_1", "global.pane_2", "global.pane_3", "global.pane_4"}
	names := [numPanes]string{paneTitles[pinnedPane], "List", paneTitles[readmePane], paneTitles[calendarPane]}
	for i, a := range actions {
		k.Panes[i] = page.Binding(a, names[i])
	}
	k.Jump = ui.Jump(k.Panes[:]...)

	// The page matches these keys first, so the list gets only the keys it
	// leaves it.
	k.feed = feed.NewKeyMap(list)
	k.cal = calendar.NewKeyMap(ui.In(keys, paneContext[calendarPane]))
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

// KeyLayers implements ui.Keyed: the keys of the page, which work in every
// pane, and then those of the focused pane, named for what they do there,
// with those of the list on view, the README or the calendar, whichever
// has the focus.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys.state(s)
	screen := ui.ContextLayer(ctxPage,
		[]key.Binding{k.Next, k.Prev, k.Zoom, k.Back, k.Refresh, k.Jump},
		[]key.Binding{k.Next, k.Jump, k.Zoom, k.Back, k.Refresh})
	focus := pinnedPane
	if s.page != nil {
		focus = s.page.focus
	}
	ctx := paneContext[focus]
	own := keyhelp.Layer{Bindings: k.paneKeys(focus), Short: k.paneShort(focus)}
	if l := s.page.list(); l != nil && focus == listPane {
		return []keyhelp.Layer{screen, ui.MergeLayers(ctx, own, keyhelp.FromHelp("", l.feed().KeyMap(), false))}
	}
	if side, ok := s.sideLayer(); ok {
		return []keyhelp.Layer{screen, ui.MergeLayers(ctx, own, side)}
	}
	return []keyhelp.Layer{screen, ui.MergeLayers(ctx, own)}
}

// paneKeys returns the keys that work in the pane, in the order the page
// matches them.
func (k KeyMap) paneKeys(p paneID) []key.Binding {
	switch p {
	case pinnedPane:
		return []key.Binding{k.Left, k.Right, k.Up, k.Down, k.Select, k.Open}
	case listPane:
		return []key.Binding{k.Select, k.Open, k.NextTab, k.PrevTab, k.ClearFilter, k.Filter, k.Sort}
	case readmePane:
		return []key.Binding{k.Open}
	case calendarPane, numPanes:
	}
	return nil
}

// paneShort returns the keys of the pane worth a hint.
func (k KeyMap) paneShort(p paneID) []key.Binding {
	switch p {
	case pinnedPane:
		return []key.Binding{k.Up, k.Down, k.Left, k.Right, k.Select, k.Open}
	case listPane:
		return []key.Binding{k.Select, k.Filter, k.Sort, k.ClearFilter, k.Open, k.NextTab}
	case readmePane:
		return []key.Binding{k.Open}
	case calendarPane, numPanes:
	}
	return nil
}

// state returns k as the page takes it with the focus on its pane: the
// keys of that pane, zoom while every pane fits, and the way back while
// there is one.
func (k KeyMap) state(s *Section) KeyMap {
	focus := pinnedPane
	if s.page != nil {
		focus = s.page.focus
	}
	for _, b := range []*key.Binding{&k.Left, &k.Right, &k.Up, &k.Down, &k.Select, &k.Open, &k.NextTab, &k.PrevTab, &k.ClearFilter, &k.Filter, &k.Sort} {
		b.SetEnabled(b.Enabled() && s.page != nil)
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
	k.Back.SetHelp(k.Back.Help().Key, "unzoom")
	k.Back.SetEnabled(k.Back.Enabled() && s.zoomed())
	return k
}
