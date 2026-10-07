package search

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// KeyMap holds the keys of the search page. While the query has the
// focus, only the keys that type nothing act; the rest edit the query.
type KeyMap struct {
	// Next and Prev move the focus between the query, the kinds and the
	// results, and Panes focus the one with that number. The query types
	// digits, so they work from the kinds and the results.
	Next  key.Binding
	Prev  key.Binding
	Panes [numAreas]key.Binding
	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding
	// Select searches from the query, and opens the result under the
	// cursor.
	Select key.Binding
	// Open opens the result under the cursor in the browser.
	Open key.Binding
	// Checks opens the pull request under the cursor on its checks.
	Checks key.Binding
	// Back goes back to the screen before the search.
	Back key.Binding
	// Filter and Sort open the filter of the kind on view on its Filters
	// and Sort tabs.
	Filter key.Binding
	Sort   key.Binding
	// KindsFilter and KindsSort do the same from the kinds, with the keys
	// of that pane.
	KindsFilter key.Binding
	KindsSort   key.Binding
	Refresh     key.Binding
	Up          key.Binding
	Down        key.Binding
	Left        key.Binding
	Right       key.Binding

	// feed is the navigation of the results, which gets the keys above
	// only if the page leaves them.
	feed feed.KeyMap
}

// areaTitles names the parts of the page in help.
var areaTitles = [numAreas]string{"query", "kinds", "results"}

// focusOf returns the part of the page that msg focuses, or -1.
func (k KeyMap) focusOf(msg tea.KeyPressMsg) area {
	for i, b := range k.Panes {
		if key.Matches(msg, b) {
			return area(i)
		}
	}
	return -1
}

func newKeyMap(keys config.Keymap) KeyMap {
	page, results, kinds := ui.In(keys, "search"), ui.In(keys, "search_results"), ui.In(keys, "search_kinds")
	k := KeyMap{
		Next:        page.Binding("global.next_pane", "next"),
		Prev:        page.Binding("global.prev_pane", "previous"),
		Select:      page.Binding("global.select", "open"),
		Open:        page.Binding("global.open", "browser"),
		Checks:      results.Binding("checks", "checks"),
		Back:        page.Binding("global.dismiss", "back"),
		Filter:      results.Binding("filter", "filter"),
		Sort:        results.Binding("sort", "sort"),
		KindsFilter: kinds.Binding("filter", "filter"),
		KindsSort:   kinds.Binding("sort", "sort"),
		Refresh:     page.Binding("global.refresh", "refresh"),
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:        key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "kinds")),
		Right:       key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "results")),
	}
	for i, a := range [numAreas]string{"global.pane_1", "global.pane_2", "global.pane_3"} {
		k.Panes[i] = page.Binding(a, areaTitles[i])
	}
	k.Jump = ui.Jump(k.Panes[:]...)
	// The page matches these keys first, so the results get only the keys
	// it leaves them, such as f, which pages down there.
	f := feed.DefaultKeyMap()
	f.Retry = key.NewBinding(key.WithKeys(k.Refresh.Keys()...), key.WithHelp(k.Refresh.Help().Key, "retry"), key.WithDisabled())
	k.feed = f
	return k
}

// arrows keeps the arrow of b, for while the query has the focus.
var (
	arrowUp   = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "kinds"))
	arrowDown = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "results"))
)

// own returns the keys of the page, in the order it matches them.
func (k KeyMap) own() []key.Binding {
	return []key.Binding{
		k.Back, k.Select, k.Next, k.Prev, k.Jump, k.Left, k.Right, k.Up, k.Down,
		k.Open, k.Checks, k.Refresh, k.Filter, k.Sort,
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Checks, k.Open, k.Filter, k.Sort, k.Left, k.Next, k.Back}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.own(), {k.KindsFilter, k.KindsSort}}
}

// KeyLayers implements ui.Keyed: the keys of the part of the page that
// has the focus, named for what they do there, with those of the results
// on view. The query types what its keys don't take.
func (s *Section) KeyLayers() []keyhelp.Layer {
	if s.area == inputArea {
		// The query types first, so its keys get only those that type
		// nothing, such as tab rather than ].
		k := s.keys.inInput()
		keys := append(k.own(), arrowUp, arrowDown)
		short := []key.Binding{k.Select, arrowDown, k.Next, k.Back}
		l := ui.ContextLayer("search_query", keys, short)
		l.Typing = true
		return []keyhelp.Layer{l}
	}
	k := s.keys.state(s)
	screen := ui.ContextLayer("search", []key.Binding{k.Back, k.Next, k.Prev, k.Jump}, []key.Binding{k.Next, k.Back})
	own := keyhelp.Layer{
		Bindings: []key.Binding{k.Select, k.Left, k.Right, k.Up, k.Down, k.Open, k.Checks, k.Refresh, k.Filter, k.Sort},
		Short:    []key.Binding{k.Up, k.Down, k.Select, k.Checks, k.Open, k.Filter, k.Sort, k.Left},
	}
	if s.area == kindsArea {
		// The kinds have no result under the cursor, so no checks, and the
		// app's repo key finds nothing selected.
		kinds := keyhelp.Layer{
			Bindings: []key.Binding{k.Select, k.Left, k.Right, k.Up, k.Down, k.Open, k.Refresh, k.KindsFilter, k.KindsSort},
			Short:    []key.Binding{k.Up, k.Down, k.Select, k.Open, k.KindsFilter, k.KindsSort, k.Left},
		}
		return []keyhelp.Layer{screen, ui.MergeLayers("search_kinds", kinds)}
	}
	if s.text == "" {
		return []keyhelp.Layer{screen, ui.MergeLayers("search_results", own)}
	}
	return []keyhelp.Layer{screen, ui.MergeLayers("search_results", own, keyhelp.FromHelp("", s.feedKeys(), false))}
}

// inInput returns k as the query takes it: back, select and the moves
// between the parts of the page act, where the query leaves them a key,
// and the arrows move to the kinds and the results.
func (k KeyMap) inInput() KeyMap {
	k.Select.SetHelp(k.Select.Help().Key, "search")
	for _, b := range []*key.Binding{
		&k.Jump, &k.Left, &k.Right, &k.Up, &k.Down, &k.Open, &k.Checks, &k.Refresh, &k.Filter, &k.Sort, &k.KindsFilter, &k.KindsSort,
	} {
		b.SetEnabled(false)
	}
	return k
}

// state returns k as the kinds or the results take it, named for what
// the keys do there.
func (k KeyMap) state(s *Section) KeyMap {
	k.Sort.SetEnabled(k.Sort.Enabled() && s.kind != core.SearchCode)
	k.KindsSort.SetEnabled(k.KindsSort.Enabled() && s.kind != core.SearchCode)
	if s.area == kindsArea {
		if s.kind == core.SearchCode {
			k.Select.SetHelp(k.Select.Help().Key, "search code")
		} else {
			k.Select.SetHelp(k.Select.Help().Key, "results")
		}
		for _, b := range []*key.Binding{&k.Left, &k.Open, &k.Checks, &k.Refresh} {
			b.SetEnabled(false)
		}
		return k
	}
	if s.kind != core.SearchRepos {
		k.Select.SetHelp(k.Select.Help().Key, "preview")
	}
	k.Checks.SetEnabled(k.Checks.Enabled() && s.kind == core.SearchPulls)
	k.Right.SetEnabled(false)
	// The moves are the list's once there is a query.
	k.Up.SetEnabled(k.Up.Enabled() && s.text == "")
	k.Down.SetEnabled(k.Down.Enabled() && s.text == "")
	return k
}
