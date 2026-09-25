package search

import (
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// KeyMap holds the keys of the search page. While the query has the
// focus, only the keys that type nothing act; the rest edit the query.
type KeyMap struct {
	// Next and Prev move the focus between the query, the kinds and the
	// results.
	Next key.Binding
	Prev key.Binding
	// Select searches from the query, and opens the result under the
	// cursor.
	Select key.Binding
	// Open opens the result under the cursor in the browser.
	Open key.Binding
	// Repo shows the repository of the result under the cursor.
	Repo key.Binding
	// Back goes back to the screen before the search.
	Back    key.Binding
	Refresh key.Binding
	Up      key.Binding
	Down    key.Binding
	Left    key.Binding
	Right   key.Binding

	// feed is the navigation of the results, without the keys above.
	feed feed.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		Next:    ui.Binding(keys, config.ActionNextTab, "next"),
		Prev:    ui.Binding(keys, config.ActionPrevTab, "previous"),
		Select:  ui.Binding(keys, config.ActionSelect, "open"),
		Open:    ui.Binding(keys, config.ActionOpen, "browser"),
		Repo:    ui.Binding(keys, config.ActionGoToRepo, "repo"),
		Back:    ui.Binding(keys, config.ActionBack, "back"),
		Refresh: ui.Binding(keys, config.ActionRefresh, "refresh"),
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "kinds")),
		Right:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "results")),
	}
	own := []key.Binding{k.Select, k.Open, k.Repo, k.Refresh, k.Back, k.Next, k.Prev, k.Left, k.Right}
	f := feed.DefaultKeyMap()
	f.PageUp = free(f.PageUp, own)
	f.PageDown = free(f.PageDown, own)
	f.Home = free(f.Home, own)
	f.End = free(f.End, own)
	f.Retry = key.NewBinding(key.WithKeys(k.Refresh.Keys()...), key.WithHelp(k.Refresh.Help().Key, "retry"), key.WithDisabled())
	k.feed = f
	return k
}

// typing returns b without the keys that type text, such as "]", for
// while the query has the focus.
func typing(b key.Binding) key.Binding {
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool {
		return utf8.RuneCountInString(k) == 1 || k == "space"
	})
	if len(keys) == len(b.Keys()) {
		return b
	}
	if len(keys) == 0 {
		return key.NewBinding(key.WithDisabled())
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], b.Help().Desc))
}

// free drops the keys of b that the page binds itself.
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

// arrows keeps the arrow of b, for while the query has the focus.
var (
	arrowUp   = key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "kinds"))
	arrowDown = key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "results"))
)

// helpKeys lists the keys of the part of the page that has the focus.
type helpKeys struct {
	k    KeyMap
	area area
	kind core.SearchKind
	feed feed.KeyMap
}

// ShortHelp returns the bindings for the short help view.
func (h helpKeys) ShortHelp() []key.Binding {
	k := h.k
	switch h.area {
	case inputArea:
		sel := typing(k.Select)
		sel.SetHelp(sel.Help().Key, "search")
		return []key.Binding{sel, arrowDown, typing(k.Next), typing(k.Back)}
	case kindsArea:
		sel := k.Select
		if h.kind == core.SearchCode {
			sel.SetHelp(sel.Help().Key, "search code")
		} else {
			sel.SetHelp(sel.Help().Key, "results")
		}
		return []key.Binding{k.Up, k.Down, sel, k.Next, k.Back}
	default:
		sel := k.Select
		if h.kind != core.SearchRepos {
			sel.SetHelp(sel.Help().Key, "preview")
		}
		return []key.Binding{k.Up, k.Down, sel, k.Repo, k.Open, k.Left, k.Back, h.feed.Retry}
	}
}

// FullHelp returns the bindings for the full help view.
func (h helpKeys) FullHelp() [][]key.Binding {
	groups := [][]key.Binding{h.ShortHelp()}
	if h.area == resultsArea {
		groups = append(groups, []key.Binding{h.feed.PageUp, h.feed.PageDown, h.feed.Home, h.feed.End, h.k.Refresh})
	}
	return append(groups, []key.Binding{h.k.Next, h.k.Prev})
}
