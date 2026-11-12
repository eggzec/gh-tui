package issues

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// keyMap holds the keys of the section and of the bubbles it shows. The
// section and its modal match their own keys first, so the bubbles get
// only the keys they leave them.
type keyMap struct {
	Select key.Binding
	Back   key.Binding
	// Filter opens the filter modal and Sort the sort modal, which the app
	// does, so the section only keeps them from the feed and shows them in
	// help.
	Filter      key.Binding
	Sort        key.Binding
	ClearFilter key.Binding
	// ToggleFilter switches the filter off, and back on.
	ToggleFilter key.Binding
	// ToggleSort switches the sort off, and back on.
	ToggleSort key.Binding
	// NextTab and PrevTab switch the state shown.
	NextTab key.Binding
	PrevTab key.Binding
	Refresh key.Binding
	Close   key.Binding `menu:"item"`
	Reopen  key.Binding `menu:"item"`
	Comment key.Binding
	Label   key.Binding
	// Edit rewrites the viewer's comment under the cursor.
	Edit key.Binding
	// Delete asks to delete the viewer's comment under the cursor.
	Delete key.Binding
	// React picks the reactions the viewer gives the comment under the
	// cursor.
	React key.Binding
	// References shows the issues and pull requests linked to the issue, in
	// a step of its modal. The list has no such key.
	References key.Binding

	// assign picks the assignees of the issue, on the Assignees group of the
	// Involved pane.
	Assign key.Binding
	// confirm answers the question that close and reopen ask.
	confirm confirm.KeyMap
	// composer are the keys of the comment composer.
	composer composer.KeyMap
	// pick are the keys of the reaction picker.
	pick picker.KeyMap
	feed feed.KeyMap
	// mark is the key that marks rows of the list.
	mark feed.MarkKeys
	// search are the keys of the prompt of the list's find and filter.
	search cmdline.KeyMap
	// tree are the keys of the conversation's rows.
	tree treeview.KeyMap
	// owner shows the author's page from the modal. The list leaves the
	// key to the app, which does it from the selection, so only the
	// modal's help lists it.
	owner key.Binding
	// panes are the keys that move between the panes of the modal.
	panes paneKeys
	// descEdit are the keys of the editor of the title and the description.
	descEdit prpanes.DescEditKeys
}

// The contexts of the keys of issues: the list, and the modal of one.
const (
	ctxList  = "issues"
	ctxModal = "issue_modal"
	// ctxDescEdit is the editor of the title and the description, whose keys
	// are control keys, since the letters are typed.
	ctxDescEdit = "issue_desc_edit"
)

// newKeyMap returns the keys of the list of issues; the modal takes a copy
// made by forModal.
func newKeyMap(keys config.Keymap) keyMap {
	list := ui.In(keys, ctxList)
	k := keyMap{
		Select:       list.Binding("global.select", "open"),
		Back:         list.Binding("global.dismiss", "close"),
		Filter:       list.Binding("filter", "filter"),
		Sort:         list.Binding("sort", "sort"),
		ClearFilter:  list.Binding("clear_filter", "clear filters"),
		ToggleFilter: list.Binding("filter_toggle", "filter on/off"),
		ToggleSort:   list.Binding("sort_toggle", "sort on/off"),
		NextTab:      list.Binding("global.next_tab", "next state"),
		PrevTab:      list.Binding("global.prev_tab", "previous state"),
		Refresh:      list.Binding("global.refresh", "refresh"),
		Close:        list.Binding("close", "close issue"),
		Reopen:       list.Binding("reopen", "reopen issue"),
		Comment:      ui.In(keys, ctxModal).Binding("comment", "comment"),
		Label:        ui.In(keys, ctxModal).Binding("labels", "labels"),
		Edit:         ui.In(keys, ctxModal).Binding("edit", "edit comment"),
		Delete:       ui.In(keys, ctxModal).Binding("delete", "delete comment"),
		React:        ui.In(keys, ctxModal).Binding("react", "react"),
		References:   ui.In(keys, ctxModal).Binding("references", "linked items"),
		Assign:       ui.In(keys, ctxSideInvolved).Binding("edit", "assignees"),
		pick:         picker.NewKeyMap(ui.Lookup(keys, "picker")),
		confirm:      confirm.NewKeyMap(ui.Lookup(keys, "confirm")),
		composer:     composer.NewKeyMap(ui.Lookup(keys, "composer")),
		owner:        list.Binding("global.owner", "author"),
		// The section and the modal match their own keys first, so the feed
		// and the thread get only the keys they leave them.
		feed:   feed.NewKeyMap(list),
		mark:   feed.NewMarkKeys(list),
		search: ui.SearchPromptKeys(keys),
		tree:   treeview.NewKeyMap(convLookup{Context: ui.In(keys, ctxModal), keys: keys}),
	}
	return k
}

// convLookup gives the actions the modal's context has. The tree has others,
// such as marking a row, that have no key here and no line in the config.
type convLookup struct {
	ui.Context
	keys config.Keymap
}

// Of implements keymap.Lookup. It asks Has, not Of, for what the config may
// lack, so that the actions the code reads stay those the config has.
func (l convLookup) Of(action string) []string {
	if !strings.Contains(action, ".") && !l.keys.Has(l.Scope()+"."+action) {
		return nil
	}
	return l.Context.Of(action)
}

// forModal returns k with the keys of the changes that the modal of an
// issue takes, which are its own.
func (k keyMap) forModal(keys config.Keymap) keyMap {
	modal := ui.In(keys, ctxModal)
	k.Close = modal.Binding("close", "close issue")
	k.Reopen = modal.Binding("reopen", "reopen issue")
	k.References = modal.Binding("references", "linked items")
	k.panes = newPaneKeys(keys)
	k.descEdit = prpanes.NewDescEditKeys(keys, ctxDescEdit)
	return k
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Back, k.Filter, k.ClearFilter, k.NextTab, k.Comment, k.Label, k.Close, k.Reopen}
}

// FullHelp implements help.KeyMap: the keys the modal matches, and then
// the list's.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Back, k.Comment, k.Label, k.Edit, k.Assign, k.Delete, k.React, k.References, k.Close, k.Reopen, k.Refresh},
		{k.Select, k.NextTab, k.PrevTab, k.ClearFilter, k.ToggleFilter, k.Filter, k.Sort, k.ToggleSort},
	}
}

// KeyLayers implements ui.Keyed: the keys of the list, with those of the
// feed. The modal of an open issue lists its own. Without a repository,
// or with issues turned off, no key does anything.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys.onList(s)
	own := keyhelp.Layer{Bindings: slices.Concat(k.FullHelp()...), Short: k.ShortHelp()}
	own.SetMenu(keymap.Menu(k))
	ui.MarkedMenu(&own, s.list.Marks())
	if !s.hasRepo || s.issuesOff() {
		return []keyhelp.Layer{ui.Off(ui.MergeLayers(ctxList, own))}
	}
	list, prompting := ui.FeedLayer(s.list)
	if prompting {
		return []keyhelp.Layer{list}
	}
	return []keyhelp.Layer{ui.MergeLayers(ctxList, own, list)}
}

// Capturing implements ui.Capturer: the list takes every key while its
// find or filter prompt is open.
func (s *Section) Capturing() bool { return s.list.Capturing() }

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
	for _, b := range []*key.Binding{&k.Back, &k.Comment, &k.Label, &k.References, &k.Assign, &k.Edit, &k.Delete, &k.React} {
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
		return []keyhelp.Layer{ui.ConfirmLayer(m.keys.confirm)}
	case m.refs != nil:
		return m.refs.KeyLayers()
	case m.over != nil:
		return m.over.Layers()
	}
	k := m.keys
	g, it := m.gate(), &m.issue
	k.Close.SetEnabled(k.Close.Enabled() && m.loaded && m.issue.State == core.StateOpen)
	k.Reopen.SetEnabled(k.Reopen.Enabled() && m.loaded && m.issue.State != core.StateOpen)
	k.Comment.SetEnabled(k.Comment.Enabled() && m.loaded)
	k.Label.SetEnabled(k.Label.Enabled() && m.loaded)
	k.References.SetEnabled(k.References.Enabled() && m.newRefs != nil)
	onConv := m.loaded && m.nav.Focus() == paneConversation
	onDesc := m.loaded && m.nav.Focus() == paneDescription
	k.Edit.SetEnabled(k.Edit.Enabled() && (onConv || onDesc))
	if onDesc {
		k.Edit.SetHelp(k.Edit.Help().Key, "edit description")
		k.Edit = g.Gated(k.Edit, ui.ActDraft, it)
	}
	onPeople := m.loaded && m.side != nil && m.nav.Focus() == paneInvolved && m.side.inv.OnAssignees()
	k.Assign.SetEnabled(k.Assign.Enabled() && onPeople)
	k.Assign = g.Gated(k.Assign, ui.ActAssign, it)
	k.Delete.SetEnabled(k.Delete.Enabled() && onConv)
	k.React.SetEnabled(k.React.Enabled() && onConv)
	owner := m.keys.owner
	owner.SetEnabled(owner.Enabled() && ui.Author(m.issue.Author) != "")
	k.Close, k.Reopen = g.Gated(k.Close, ui.ActClose, it), g.Gated(k.Reopen, ui.ActReopen, it)
	k.Comment, k.Label = g.Gated(k.Comment, ui.ActComment, it), g.Gated(k.Label, ui.ActLabel, it)
	for _, b := range []*key.Binding{&k.Select, &k.NextTab, &k.PrevTab, &k.ClearFilter, &k.Filter, &k.Sort} {
		b.SetEnabled(false)
	}
	own := keyhelp.FromHelp("", k, false)
	own.Bindings = append(own.Bindings, owner)
	pk := m.keys.panes
	can := m.canDivide()
	pk.dividerLeft.SetEnabled(pk.dividerLeft.Enabled() && can)
	pk.dividerRight.SetEnabled(pk.dividerRight.Enabled() && can)
	pk.dividerReset.SetEnabled(pk.dividerReset.Enabled() && can && m.dividers.Moved())
	stack := m.canStack()
	pk.dividerUp.SetEnabled(pk.dividerUp.Enabled() && stack)
	pk.dividerDown.SetEnabled(pk.dividerDown.Enabled() && stack)
	own.Bindings = append(own.Bindings, pk.bindings()...)
	// The conversation is the one pane of the modal. Its actions that the
	// config has no key for here, such as marking a row, are left out.
	tree := keyhelp.FromHelp("", m.keys.tree, false)
	tree.Bindings = slices.DeleteFunc(tree.Bindings, func(b key.Binding) bool {
		return len(b.Keys()) == 0 && len(keymap.Actions(b)) == 0
	})
	return []keyhelp.Layer{ui.MergeLayers(ctxModal, own, tree)}
}
