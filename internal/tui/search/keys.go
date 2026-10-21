package search

import (
	"slices"
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// KeyMap holds the keys of the search page. While the query types, only
// the keys that type nothing act; the rest edit the query.
type KeyMap struct {
	// Next and Prev move the focus between the query, the results and the
	// preview, and Kinds show the kind with that number, in the results.
	Next  key.Binding
	Prev  key.Binding
	Kinds [maxKinds]key.Binding
	// Jump holds the keys of Kinds, which it stands for in help.
	Jump key.Binding
	// Insert starts typing in the query.
	Insert key.Binding
	// NextTab and PrevTab show the next and the previous kind of results.
	NextTab key.Binding
	PrevTab key.Binding
	// Select searches from the query, and opens the result under the
	// cursor.
	Select key.Binding
	// Open opens the result under the cursor in the browser.
	Open key.Binding
	// Checks opens the pull request under the cursor on its checks.
	Checks key.Binding
	// Filter and Sort open the filter of the kind on view on its Filters
	// and Sort tabs.
	Filter  key.Binding
	Sort    key.Binding
	Refresh key.Binding
	// GroupBy opens the choice of the levels the repositories are grouped
	// by, and ToggleGroups switches the grouping off, and back on.
	GroupBy      key.Binding
	ToggleGroups key.Binding
	// NextHit and PrevHit move the cursor of the results by one hit.
	NextHit key.Binding
	PrevHit key.Binding

	// order are the keys of the picker of the levels to group by.
	order orderpick.KeyMap
	// query holds the keys of the query while it types.
	query queryKeys
	// feed is the navigation of the results, which gets the keys above
	// only if the page leaves them.
	feed feed.KeyMap
	// search are the keys of the prompt of the results' find and filter.
	search cmdline.KeyMap
}

// queryKeys are the keys of the query while it types: those that type
// nothing.
type queryKeys struct {
	// Submit searches, and Cancel stops typing and moves to the results.
	Submit key.Binding `keymap:"submit" help:"search"`
	Cancel key.Binding `keymap:"cancel" help:"results"`
}

// maxKinds is how many kinds the digits reach: pane_1 to pane_6 of the
// global keys, and kind_7 of the search modal.
const maxKinds = 7

// kindOf returns the position of the kind that msg shows, or -1.
func (k KeyMap) kindOf(msg tea.KeyPressMsg) int {
	for i, b := range k.Kinds {
		if keymap.Matches(msg, b) {
			return i
		}
	}
	return -1
}

func newKeyMap(keys config.Keymap) KeyMap {
	page, results := ui.In(keys, "search_modal"), ui.In(keys, "search_results")
	k := KeyMap{
		Next:    page.Binding("global.next_pane", "next"),
		Prev:    page.Binding("global.prev_pane", "previous"),
		Insert:  page.Binding("insert", "type"),
		NextTab: page.Binding("global.next_tab", "next kind"),
		PrevTab: page.Binding("global.prev_tab", "previous kind"),
		Select:  page.Binding("global.select", "open"),
		Open:    page.Binding("global.open", "browser"),
		Checks:  results.Binding("checks", "checks"),
		Filter:  results.Binding("filter", "filter"),
		Sort:    results.Binding("sort", "sort"),
		Refresh: page.Binding("global.refresh", "refresh"),

		GroupBy:      results.Binding("group_by", "group by"),
		ToggleGroups: results.Binding("group_toggle", "group on/off"),
		NextHit:      results.Binding("next_hit", "next hit"),
		PrevHit:      results.Binding("prev_hit", "previous hit"),

		order: ui.OrderPickKeys(keys),
		// The page matches these keys first, so the results get only the keys
		// it leaves them.
		feed:   feed.NewKeyMap(results),
		search: ui.SearchPromptKeys(keys),
	}
	for i := range k.Kinds {
		a := "global.pane_" + strconv.Itoa(i+1)
		if i == maxKinds-1 {
			a = "kind_7"
		}
		k.Kinds[i] = page.Binding(a, "kind")
		// A digit with no kind does nothing, and is not in help.
		k.Kinds[i].SetEnabled(k.Kinds[i].Enabled() && i < len(kinds))
	}
	k.Jump = ui.Jump(k.Kinds[:]...)
	k.Jump.SetHelp(k.Jump.Help().Key, "show kind")
	keymap.Fill(&k.query, ui.Lookup(keys, "search_query"))
	return k
}

// own returns the keys of the page, in the order it matches them.
func (k KeyMap) own() []key.Binding {
	return []key.Binding{
		k.Insert, k.NextTab, k.PrevTab, k.Select, k.Next, k.Prev, k.Jump,
		k.Open, k.Checks, k.Refresh, k.Filter, k.Sort, k.GroupBy, k.ToggleGroups,
		k.NextHit, k.PrevHit,
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Insert, k.Select, k.Checks, k.Open, k.Filter, k.Sort, k.NextTab, k.Next}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.own()}
}

// KeyLayers implements ui.Keyed: the keys of the part of the page that
// has the focus, named for what they do there, with those of the results
// on view. The query types what its keys don't take.
func (s *Section) KeyLayers() []keyhelp.Layer {
	if list, prompting := s.feedLayer(); prompting {
		return []keyhelp.Layer{list}
	}
	if s.typing {
		// The query types first, so it has only the keys that type
		// nothing.
		k := s.keys.query
		l := ui.ContextLayer("search_query", []key.Binding{k.Submit, k.Cancel}, []key.Binding{k.Submit, k.Cancel})
		l.Typing = true
		return []keyhelp.Layer{l}
	}
	k := s.keys.state(s)
	screen := []key.Binding{k.Insert, k.NextTab, k.PrevTab, k.Next, k.Prev, k.Jump}
	if s.area == inputArea {
		// The query has no pane keys of its own: what acts on a result or
		// the list waits for the results.
		screen = append(screen, k.Select, k.Refresh)
		return []keyhelp.Layer{ui.ContextLayer("search_modal", screen, []key.Binding{k.Insert, k.Select, k.NextTab, k.Next})}
	}
	// Without a query the results are the page's own suggestions, which
	// move with the keys of the list; with one, the list's layer has them.
	var moves []key.Binding
	if s.text == "" {
		moves = []key.Binding{k.feed.Up, k.feed.Down}
	}
	own := keyhelp.Layer{
		Bindings: slices.Concat([]key.Binding{k.Select}, moves, []key.Binding{k.Open, k.Checks, k.Refresh, k.Filter, k.Sort, k.GroupBy, k.ToggleGroups}),
		Short:    slices.Concat(moves, []key.Binding{k.Select, k.Checks, k.Open, k.Filter, k.Sort}),
	}
	page := ui.ContextLayer("search_modal", screen, []key.Binding{k.Insert, k.NextTab, k.Next})
	if s.text == "" {
		return []keyhelp.Layer{page, ui.MergeLayers("search_results", own)}
	}
	list, _ := s.feedLayer()
	return []keyhelp.Layer{page, ui.MergeLayers("search_results", own, list)}
}

// state returns k as the query or the results take it, named for what the
// keys do there.
func (k KeyMap) state(s *Section) KeyMap {
	k.Sort.SetEnabled(k.Sort.Enabled() && s.kind != core.SearchCode)
	if s.area == inputArea {
		k.Select.SetHelp(k.Select.Help().Key, "search")
		return k
	}
	_, codeShown := s.visibleCode()
	switch {
	case s.kind == core.SearchCode && s.text != "" && !codeShown:
		k.Select.SetHelp(k.Select.Help().Key, "search code")
	case s.kind != core.SearchRepos && s.text != "":
		k.Select.SetHelp(k.Select.Help().Key, "preview")
	}
	k.Checks.SetEnabled(k.Checks.Enabled() && s.kind == core.SearchPulls)
	repos := s.kind == core.SearchRepos
	k.GroupBy.SetEnabled(k.GroupBy.Enabled() && repos)
	k.ToggleGroups.SetEnabled(k.ToggleGroups.Enabled() && repos && len(s.grouping.Levels) > 0)
	return k
}
