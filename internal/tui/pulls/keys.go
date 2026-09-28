package pulls

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
)

// keyMap holds the keys of the section and of the bubbles it shows. The
// section and its modal match their own keys first, so the bubbles get
// only the keys they leave them.
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

	// confirm answers the question that merge, close and reopen ask.
	confirm ui.ConfirmKeys
	feed    feed.KeyMap
	thread  thread.KeyMap
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
		confirm:     ui.DefaultConfirmKeys(),
	}
	// The section and the modal match their own keys first, so the feed
	// and the thread get only the keys they leave them, such as f, which
	// pages down there and opens the filter here.
	f := feed.DefaultKeyMap()
	// Refresh fetches failed chunks again too, so the feed's error row names
	// its keys.
	f.Retry = retry(k.Refresh)
	k.feed = f

	t := thread.DefaultKeyMap()
	t.Toggle = ui.Binding(keys, config.ActionSelect, t.Toggle.Help().Desc)
	// The thread offers retry itself once something failed.
	t.Retry = retry(k.Refresh)
	t.Retry.SetEnabled(k.Refresh.Enabled())
	k.thread = t
	return k
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

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Back, k.Filter, k.ClearFilter, k.NextTab, k.Merge, k.Close, k.Reopen, k.Checks, k.Open}
}

// FullHelp implements help.KeyMap: the changes, which the section and the
// modal match first, and then the rest.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Merge, k.Close, k.Reopen, k.ToggleDraft},
		{k.Back, k.Select, k.Checks, k.NextTab, k.PrevTab, k.ClearFilter, k.Refresh, k.Open, k.Filter, k.Sort},
	}
}

// KeyLayers implements ui.Keyed: the keys of the list, and then those of
// the feed. The modal of an open pull request lists its own. Without a
// repository, no key does anything.
func (s *Section) KeyLayers() []keyhelp.Layer {
	own := keyhelp.FromHelp(ui.PullsTitle, s.keys.onList(s), false)
	if !s.hasRepo || s.feed == nil {
		return []keyhelp.Layer{ui.Off(own)}
	}
	return []keyhelp.Layer{own, keyhelp.FromHelp("list", s.feed.KeyMap(), false)}
}

// onList returns k as the list takes it: the changes that apply to the pull
// request under the cursor, and the clear key while a filter is in
// force. Back is the modal's.
func (k keyMap) onList(s *Section) keyMap {
	pr, ok := s.target()
	k = k.withChanges(s.gate(), s.mergeMethod, pr, ok)
	k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.query != "")
	k.Back.SetEnabled(false)
	return k
}
