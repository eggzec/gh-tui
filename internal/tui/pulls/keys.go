package pulls

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
)

// keyMap holds the keys of the section, and those of its bubbles without
// the keys the section takes for itself.
type keyMap struct {
	Select key.Binding
	Back   key.Binding
	// Filter and Sort open the filter modal on its Filters and Sort tabs,
	// which the app does, so the section only keeps them from the feed and
	// shows them in help.
	Filter      key.Binding
	Sort        key.Binding
	ClearFilter key.Binding
	// NextTab and PrevTab switch the state shown.
	NextTab key.Binding
	PrevTab key.Binding
	Refresh key.Binding
	Open    key.Binding

	Merge       key.Binding
	Close       key.Binding
	Reopen      key.Binding
	ToggleDraft key.Binding
	// Checks shows the checks of a pull request, in a step of its modal.
	Checks key.Binding

	feed   feed.KeyMap
	thread thread.KeyMap
}

func newKeyMap(keys map[string][]string) keyMap {
	k := keyMap{
		Select:      ui.Binding(keys, config.ActionSelect, "open"),
		Back:        ui.Binding(keys, config.ActionBack, "back"),
		Filter:      ui.Binding(keys, config.ActionFilter, "filter"),
		Sort:        ui.Binding(keys, config.ActionSort, "sort"),
		ClearFilter: ui.Binding(keys, config.ActionClearFilter, "clear filters"),
		NextTab:     ui.Binding(keys, config.ActionNextFilter, "next state"),
		PrevTab:     ui.Binding(keys, config.ActionPrevFilter, "previous state"),
		Refresh:     ui.Binding(keys, config.ActionRefresh, "refresh"),
		Open:        ui.Binding(keys, config.ActionOpen, "open in browser"),

		Merge:       ui.Binding(keys, config.ActionMerge, "merge"),
		Close:       ui.Binding(keys, config.ActionClose, "close"),
		Reopen:      ui.Binding(keys, config.ActionReopen, "reopen"),
		ToggleDraft: ui.Binding(keys, config.ActionToggleDraft, "convert to draft"),
		Checks:      ui.Binding(keys, config.ActionChecks, "checks"),
	}
	own := k.list()
	f := feed.DefaultKeyMap()
	f.Up, f.Down = without(f.Up, own), without(f.Down, own)
	f.PageUp, f.PageDown = without(f.PageUp, own), without(f.PageDown, own)
	f.Home, f.End = without(f.Home, own), without(f.End, own)
	// Refresh fetches failed chunks again too, so the feed's error row names
	// its keys.
	f.Retry = retry(k.Refresh)
	k.feed = f

	own = k.detail()
	t := thread.DefaultKeyMap()
	t.Up, t.Down = without(t.Up, own), without(t.Down, own)
	t.PageUp, t.PageDown = without(t.PageUp, own), without(t.PageDown, own)
	t.HalfPageUp, t.HalfPageDown = without(t.HalfPageUp, own), without(t.HalfPageDown, own)
	t.Top, t.Bottom = without(t.Top, own), without(t.Bottom, own)
	// The thread offers retry itself once something failed.
	t.Retry = retry(k.Refresh)
	t.Retry.SetEnabled(k.Refresh.Enabled())
	k.thread = t
	return k
}

// list returns the bindings the section handles before the feed.
func (k keyMap) list() []key.Binding {
	return []key.Binding{k.Select, k.Filter, k.Sort, k.ClearFilter, k.NextTab, k.PrevTab, k.Refresh, k.Open, k.Merge, k.Close, k.Reopen, k.ToggleDraft, k.Checks}
}

// detail returns the bindings the modal handles before the thread.
func (k keyMap) detail() []key.Binding {
	return []key.Binding{k.Back, k.Refresh, k.Open, k.Merge, k.Close, k.Reopen, k.ToggleDraft, k.Checks}
}

// retry returns the refresh keys as a retry binding that starts disabled, so
// that a bubble enables it only while something failed.
func retry(refresh key.Binding) key.Binding {
	if !refresh.Enabled() {
		return key.NewBinding(key.WithDisabled())
	}
	return key.NewBinding(
		key.WithKeys(refresh.Keys()...),
		key.WithHelp(refresh.Help().Key, "retry"),
		key.WithDisabled(),
	)
}

// without returns b without the keys of taken, relabelled with what it has
// left, so that help never offers a key that does something else here.
func without(b key.Binding, taken []key.Binding) key.Binding {
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
	labels := make([]string, 0, 2)
	for _, k := range keys[:min(len(keys), 2)] {
		labels = append(labels, keyLabel(k))
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(labels, "/"), b.Help().Desc))
}

func keyLabel(k string) string {
	switch k {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "pgdown":
		return "pgdn"
	}
	return k
}

// keyHelp is a help.KeyMap made of fixed lists.
type keyHelp struct {
	short []key.Binding
	full  [][]key.Binding
}

func (h keyHelp) ShortHelp() []key.Binding  { return h.short }
func (h keyHelp) FullHelp() [][]key.Binding { return h.full }

// Help implements ui.Section. It lists the keys of the list; the modal of
// an open pull request lists its own.
func (s *Section) Help() help.KeyMap {
	if !s.hasRepo {
		return keyHelp{}
	}
	k, f := s.keys, s.keys.feed
	pr, ok := s.target()
	changes := k.changeHelp(s.gate(), s.mergeMethod, pr, ok)
	merge, closing, reopen := changes[0], changes[1], changes[2]
	k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.query != "")
	return keyHelp{
		short: []key.Binding{f.Up, f.Down, k.Select, k.Filter, k.ClearFilter, k.NextTab, merge, closing, reopen, k.Checks, k.Open},
		full: [][]key.Binding{
			{f.Up, f.Down, f.PageUp, f.PageDown},
			{f.Home, f.End},
			{k.Select, k.Filter, k.Sort, k.ClearFilter, k.NextTab, k.PrevTab, k.Refresh, k.Checks, k.Open},
			changes,
		},
	}
}
