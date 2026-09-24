package issues

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
)

// keyMap holds the keys of the section and of the bubbles it shows. The
// section handles its own keys first, so the bubbles' keys leave out any the
// section takes in the same view.
type keyMap struct {
	Select  key.Binding
	Back    key.Binding
	Filter  key.Binding
	Refresh key.Binding
	Open    key.Binding
	Close   key.Binding
	Reopen  key.Binding
	Comment key.Binding
	Label   key.Binding

	feed   feed.KeyMap
	thread thread.KeyMap
}

func newKeyMap(keys map[string][]string) keyMap {
	k := keyMap{
		Select:  ui.Binding(keys, config.ActionSelect, "open"),
		Back:    ui.Binding(keys, config.ActionBack, "back"),
		Filter:  ui.Binding(keys, config.ActionFilter, "filter"),
		Refresh: ui.Binding(keys, config.ActionRefresh, "refresh"),
		Open:    ui.Binding(keys, config.ActionOpen, "browser"),
		Close:   ui.Binding(keys, config.ActionClose, "close"),
		Reopen:  ui.Binding(keys, config.ActionReopen, "reopen"),
		Comment: ui.Binding(keys, config.ActionComment, "comment"),
		Label:   ui.Binding(keys, config.ActionLabel, "labels"),
	}

	fk := feed.DefaultKeyMap()
	listKeys := []key.Binding{k.Select, k.Filter, k.Refresh, k.Open, k.Close, k.Reopen}
	for _, b := range []*key.Binding{&fk.Up, &fk.Down, &fk.PageUp, &fk.PageDown, &fk.Home, &fk.End} {
		*b = without(*b, listKeys)
	}
	// Refresh reloads failed pages too, so it doubles as retry.
	fk.Retry = retry(k.Refresh)
	fk.Retry.SetEnabled(false)
	k.feed = fk

	tk := thread.DefaultKeyMap()
	detailKeys := []key.Binding{k.Back, k.Refresh, k.Open, k.Close, k.Reopen, k.Comment, k.Label}
	for _, b := range []*key.Binding{&tk.Up, &tk.Down, &tk.PageUp, &tk.PageDown, &tk.HalfPageUp, &tk.HalfPageDown, &tk.Top, &tk.Bottom} {
		*b = without(*b, detailKeys)
	}
	tk.Retry = retry(k.Refresh)
	k.thread = tk
	return k
}

// retry is the refresh binding, described as retry for the bubbles' error
// hints.
func retry(refresh key.Binding) key.Binding {
	if len(refresh.Keys()) == 0 {
		return key.NewBinding(key.WithDisabled())
	}
	return key.NewBinding(key.WithKeys(refresh.Keys()...), key.WithHelp(refresh.Help().Key, "retry"))
}

var keyLabels = strings.NewReplacer("pgdown", "pgdn", "pgup", "pgup", "down", "↓", "up", "↑")

// without returns b without the keys that taken use.
func without(b key.Binding, taken []key.Binding) key.Binding {
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool {
		return slices.ContainsFunc(taken, func(t key.Binding) bool {
			return slices.Contains(t.Keys(), k)
		})
	})
	switch len(keys) {
	case len(b.Keys()):
		return b
	case 0:
		return key.NewBinding(key.WithDisabled())
	}
	labels := make([]string, len(keys))
	for i, k := range keys {
		labels[i] = keyLabels.Replace(k)
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(labels, "/"), b.Help().Desc))
}

// Help implements ui.Section. It lists the keys of the list; the modal of
// an open issue lists its own.
func (s *Section) Help() help.KeyMap {
	k, fk := s.keys, s.keys.feed
	if !s.hasRepo {
		return keyHelp{}
	}
	// Offer close or reopen, whichever applies to the issue at hand.
	it, ok := s.target()
	k.Close.SetEnabled(k.Close.Enabled() && ok && it.State == core.StateOpen)
	k.Reopen.SetEnabled(k.Reopen.Enabled() && ok && it.State != core.StateOpen)
	return keyHelp{
		short: []key.Binding{fk.Up, fk.Down, k.Select, k.Filter, k.Close, k.Reopen, k.Open},
		full: [][]key.Binding{
			{fk.Up, fk.Down, fk.PageUp, fk.PageDown, fk.Home, fk.End},
			{k.Select, k.Filter, k.Open, k.Refresh},
			{k.Close, k.Reopen},
		},
	}
}

// Help implements ui.Modal. While the prompt is open, it lists the keys of
// the prompt.
func (m *detailModal) Help() help.KeyMap {
	if m.composing != composeNone {
		return keyHelp{short: m.prompt.ShortHelp(), full: m.prompt.FullHelp()}
	}
	k, tk := m.keys, m.keys.thread
	k.Close.SetEnabled(k.Close.Enabled() && m.loaded && m.issue.State == core.StateOpen)
	k.Reopen.SetEnabled(k.Reopen.Enabled() && m.loaded && m.issue.State != core.StateOpen)
	k.Comment.SetEnabled(k.Comment.Enabled() && m.loaded)
	k.Label.SetEnabled(k.Label.Enabled() && m.loaded)
	return keyHelp{
		short: []key.Binding{tk.Down, tk.Up, k.Back, k.Comment, k.Label, k.Close, k.Reopen, k.Open},
		full: [][]key.Binding{
			{tk.Up, tk.Down, tk.PageUp, tk.PageDown},
			{tk.HalfPageUp, tk.HalfPageDown, tk.Top, tk.Bottom},
			{k.Back, k.Comment, k.Label, k.Open, k.Refresh},
			{k.Close, k.Reopen},
		},
	}
}

// keyHelp is a help.KeyMap of fixed bindings.
type keyHelp struct {
	short []key.Binding
	full  [][]key.Binding
}

func (h keyHelp) ShortHelp() []key.Binding  { return h.short }
func (h keyHelp) FullHelp() [][]key.Binding { return h.full }
