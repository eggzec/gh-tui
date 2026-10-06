package issues

import (
	"slices"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
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
	Close   key.Binding
	Reopen  key.Binding
	Comment key.Binding
	Label   key.Binding

	// confirm answers the question that close and reopen ask.
	confirm ui.ConfirmKeys
	feed    feed.KeyMap
	thread  thread.KeyMap
	// owner shows the author's page from the modal. The list leaves the
	// key to the app, which does it from the selection, so only the
	// modal's help lists it.
	owner key.Binding
}

// The contexts of the keys of issues: the list, and the modal of one.
const (
	ctxList  = "issues"
	ctxModal = "issue_modal"
)

// newKeyMap returns the keys of the list of issues; the modal takes a copy
// made by forModal.
func newKeyMap(keys config.Keymap) keyMap {
	list := ui.In(keys, ctxList)
	k := keyMap{
		Select:      list.Binding("global.select", "open"),
		Back:        list.Binding("global.dismiss", "back"),
		Filter:      list.Binding("filter", "filter"),
		Sort:        list.Binding("sort", "sort"),
		ClearFilter: list.Binding("clear_filter", "clear filters"),
		NextTab:     list.Binding("global.next_tab", "next state"),
		PrevTab:     list.Binding("global.prev_tab", "previous state"),
		Refresh:     list.Binding("global.refresh", "refresh"),
		Open:        list.Binding("global.open", "browser"),
		Close:       list.Binding("close", "close"),
		Reopen:      list.Binding("reopen", "reopen"),
		Comment:     list.Binding("comment", "comment"),
		Label:       list.Binding("labels", "labels"),
		confirm:     ui.DefaultConfirmKeys(),
		owner:       list.Binding("global.owner", "owner page"),
	}

	// The section and the modal match their own keys first, so the feed
	// and the thread get only the keys they leave them, such as f, which
	// opens the filter here.
	fk := feed.DefaultKeyMap()
	// Refresh reloads failed pages too, so it doubles as retry.
	fk.Retry = retry(k.Refresh)
	fk.Retry.SetEnabled(false)
	k.feed = fk

	tk := thread.DefaultKeyMap()
	tk.Toggle = ui.Binding(keys, config.ActionSelect, tk.Toggle.Help().Desc)
	tk.Retry = retry(k.Refresh)
	k.thread = tk
	return k
}

// forModal returns k with the keys of the changes that the modal of an
// issue takes, which are its own.
func (k keyMap) forModal(keys config.Keymap) keyMap {
	modal := ui.In(keys, ctxModal)
	k.Close = modal.Binding("close", "close")
	k.Reopen = modal.Binding("reopen", "reopen")
	k.Comment = modal.Binding("comment", "comment")
	k.Label = modal.Binding("labels", "labels")
	return k
}

// retry is the refresh binding, described as retry for the bubbles' error
// hints. Without keys, as while refresh is unbound, it is off.
func retry(refresh key.Binding) key.Binding {
	return key.NewBinding(key.WithKeys(refresh.Keys()...), key.WithHelp(refresh.Help().Key, "retry"))
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Back, k.Filter, k.ClearFilter, k.NextTab, k.Comment, k.Label, k.Close, k.Reopen, k.Open}
}

// FullHelp implements help.KeyMap: the keys the modal matches, and then
// the list's.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Back, k.Comment, k.Label, k.Close, k.Reopen, k.Refresh, k.Open},
		{k.Select, k.NextTab, k.PrevTab, k.ClearFilter, k.Filter, k.Sort},
	}
}

// KeyLayers implements ui.Keyed: the keys of the list, with those of the
// feed. The modal of an open issue lists its own. Without a repository,
// or with issues turned off, no key does anything.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys.onList(s)
	own := keyhelp.Layer{Bindings: slices.Concat(k.FullHelp()...), Short: k.ShortHelp()}
	if !s.hasRepo || s.issuesOff() {
		return []keyhelp.Layer{ui.Off(ui.MergeLayers(ctxList, own))}
	}
	return []keyhelp.Layer{ui.MergeLayers(ctxList, own, keyhelp.FromHelp("", s.list.KeyMap(), false))}
}

// onList returns k as the list takes it: close or reopen, whichever applies
// to the issue at hand, if the viewer may, and the clear key while a
// filter is in force. The modal's own keys don't work here.
func (k keyMap) onList(s *Section) keyMap {
	it, ok := s.target()
	g := s.gate()
	k.Close.SetEnabled(k.Close.Enabled() && ok && it.State == core.StateOpen)
	k.Reopen.SetEnabled(k.Reopen.Enabled() && ok && it.State != core.StateOpen)
	k.Close, k.Reopen = g.Gated(k.Close, ui.ActClose, &it), g.Gated(k.Reopen, ui.ActReopen, &it)
	k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.query != "")
	for _, b := range []*key.Binding{&k.Back, &k.Comment, &k.Label} {
		b.SetEnabled(false)
	}
	return k
}

// KeyLayers implements ui.Keyed: the answer while a question is open, the
// prompt while it takes every key, and otherwise the modal's own keys and
// then the thread's.
func (m *detailModal) KeyLayers() []keyhelp.Layer {
	switch {
	case m.ask != nil:
		return []keyhelp.Layer{m.keys.confirm.Layer()}
	case m.composing != composeNone:
		return []keyhelp.Layer{ui.ContextHelp("prompt", m.prompt, true)}
	}
	k := m.keys
	g, it := m.gate(), &m.issue
	k.Close.SetEnabled(k.Close.Enabled() && m.loaded && m.issue.State == core.StateOpen)
	k.Reopen.SetEnabled(k.Reopen.Enabled() && m.loaded && m.issue.State != core.StateOpen)
	k.Comment.SetEnabled(k.Comment.Enabled() && m.loaded)
	k.Label.SetEnabled(k.Label.Enabled() && m.loaded)
	owner := m.keys.owner
	owner.SetEnabled(owner.Enabled() && ui.Author(m.issue.Author) != "")
	k.Close, k.Reopen = g.Gated(k.Close, ui.ActClose, it), g.Gated(k.Reopen, ui.ActReopen, it)
	k.Comment, k.Label = g.Gated(k.Comment, ui.ActComment, it), g.Gated(k.Label, ui.ActLabel, it)
	for _, b := range []*key.Binding{&k.Select, &k.NextTab, &k.PrevTab, &k.ClearFilter, &k.Filter, &k.Sort} {
		b.SetEnabled(false)
	}
	own := keyhelp.FromHelp("", k, false)
	own.Bindings = append(own.Bindings, owner)
	// The thread is the one pane of the modal.
	return []keyhelp.Layer{ui.MergeLayers(ctxModal, own, keyhelp.FromHelp("", m.thread, false))}
}
