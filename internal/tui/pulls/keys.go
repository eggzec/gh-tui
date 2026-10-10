package pulls

import (
	"slices"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
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
	// Checks shows the checks of a pull request, on a tab of its modal.
	Checks key.Binding
	// References shows the issues and pull requests linked to it, in a step
	// of its modal. The list has no such key.
	References key.Binding
	// FindFile finds one of the changed files of a pull request in a step
	// of its modal, and shows its diff. The list has no such key.
	FindFile key.Binding

	// confirm answers the question that merge, close and reopen ask.
	confirm ui.ConfirmKeys
	// finder are the keys of the step that finds a changed file.
	finder finder.KeyMap
	feed   feed.KeyMap
	// mark is the key that marks rows of the list.
	mark feed.MarkKeys
	// search are the keys of the prompt of the list's find and filter.
	search cmdline.KeyMap
	thread thread.KeyMap
	// tree moves through the tree of the changed files, and diff through
	// their diff, on the Files tab of the modal.
	tree tree.KeyMap
	diff diff.KeyMap
	// overview moves the cursor through the Attention list of the
	// Overview tab, and attend goes to the target of the row under it.
	overview ui.MoveKeys
	attend   key.Binding
	// nextPane and prevPane move the focus between the tree and the diff
	// of the Files tab, panes focus one of them, and jump stands for the
	// keys of panes in help. zoom shows the focused one alone. The modal
	// takes them from the global keys, so the list has none, and they
	// are the modal's alone to list.
	nextPane, prevPane key.Binding
	panes              [numFilesPanes]key.Binding
	jump               key.Binding
	zoom               key.Binding
	// owner shows the author's page from the modal. The list leaves the
	// key to the app, which does it from the selection, so only the
	// modal's help lists it.
	owner key.Binding
}

// The contexts of the keys of pull requests: the list, the modal of one,
// its overview and conversation, and the tree of changed files and the diff.
const (
	ctxList         = "pulls"
	ctxModal        = "pull_modal"
	ctxOverview     = "pull_overview"
	ctxConversation = "pull_conversation"
	ctxFiles        = "pull_files"
	ctxDiff         = "pull_diff"
)

// newKeyMap returns the keys of the list of pull requests, with those of
// the modal of one under the names of the list's changes, so that the
// modal takes a copy made by forModal.
func newKeyMap(keys config.Keymap) keyMap {
	list := ui.In(keys, ctxList)
	k := keyMap{
		Select:      list.Binding("global.select", "open"),
		Back:        list.Binding("global.dismiss", "close"),
		Filter:      list.Binding("filter", "filter"),
		Sort:        list.Binding("sort", "sort"),
		ClearFilter: list.Binding("clear_filter", "clear filters"),
		NextTab:     list.Binding("global.next_tab", "next state"),
		PrevTab:     list.Binding("global.prev_tab", "previous state"),
		Refresh:     list.Binding("global.refresh", "refresh"),
		Open:        list.Binding("global.open", "open in browser"),

		Merge:       list.Binding("merge", "merge"),
		Close:       list.Binding("close", "close PR"),
		Reopen:      list.Binding("reopen", "reopen PR"),
		ToggleDraft: list.Binding("draft", "convert to draft"),
		Checks:      list.Binding("checks", "checks"),
		References:  ui.In(keys, ctxModal).Binding("references", "linked items"),
		FindFile:    ui.In(keys, ctxModal).Binding("global.find_file", "find file"),
		confirm:     ui.NewConfirmKeys(keys),
		finder:      finder.NewKeyMap(ui.Lookup(keys, "finder")),
		owner:       list.Binding("global.owner", "author"),
	}
	// The section and the modal match their own keys first, so the feed
	// and the thread get only the keys they leave them.
	k.feed = feed.NewKeyMap(list)
	k.mark = feed.NewMarkKeys(list)
	k.search = ui.SearchPromptKeys(keys)

	t := thread.NewKeyMap(ui.In(keys, ctxConversation))
	// The thread offers retry itself once something failed.
	t.Retry = retry(k.Refresh)
	t.Retry.SetEnabled(k.Refresh.Enabled())
	k.thread = t

	files := ui.In(keys, ctxFiles)
	k.tree = tree.NewKeyMap(files)
	// A file shows its diff, and a directory folds.
	k.tree.Open = files.Binding("global.select", "show diff")
	k.diff = diff.NewKeyMap(ui.In(keys, ctxDiff))

	overview := ui.In(keys, ctxOverview)
	k.overview = ui.NewMoveKeys(overview)
	k.attend = overview.Binding("global.select", "go")
	return k
}

// setPaneKeys makes the keys of the panes of the Files tab, from the
// global ones, as the keys of the context c have them.
func (k *keyMap) setPaneKeys(c ui.Context) {
	k.nextPane = c.Binding("global.next_pane", "pane")
	k.prevPane = c.Binding("global.prev_pane", "previous pane")
	k.zoom = c.Binding("global.zoom", "zoom")
	for i, a := range [numFilesPanes]string{"global.pane_1", "global.pane_2"} {
		k.panes[i] = c.Binding(a, filesPaneTitles[i])
	}
	k.jump = ui.Jump(k.panes[:]...)
}

// forModal returns k with the keys of the changes and of the checks that
// the modal of a pull request takes, which are its own.
func (k keyMap) forModal(keys config.Keymap) keyMap {
	modal := ui.In(keys, ctxModal)
	k.Merge = modal.Binding("merge", "merge")
	k.Close = modal.Binding("close", "close PR")
	k.Reopen = modal.Binding("reopen", "reopen PR")
	k.ToggleDraft = modal.Binding("draft", "convert to draft")
	k.Checks = modal.Binding("checks", "checks")
	k.References = modal.Binding("references", "linked items")
	k.FindFile = modal.Binding("global.find_file", "find file")
	// The modal closes on esc, as it does from the list, and its tabs are those of the global keys.
	k.Back = modal.Binding("global.dismiss", "close")
	k.NextTab = modal.Binding("global.next_tab", "next tab")
	k.PrevTab = modal.Binding("global.prev_tab", "previous tab")
	k.setPaneKeys(modal)
	return k
}

// retry returns the refresh keys as a retry binding that starts disabled, so
// that a bubble enables it only while something failed. Without keys, as
// while refresh is unbound, it can't be enabled.
func retry(refresh key.Binding) key.Binding {
	return keymap.Derive(key.NewBinding(
		key.WithKeys(refresh.Keys()...),
		key.WithHelp(refresh.Help().Key, "retry"),
		key.WithDisabled(),
	), refresh)
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
		{k.Back, k.Select, k.Checks, k.References, k.FindFile, k.NextTab, k.PrevTab, k.ClearFilter, k.Refresh, k.Open, k.Filter, k.Sort},
	}
}

// KeyLayers implements ui.Keyed: the keys of the list, with those of the
// feed. The modal of an open pull request lists its own. Without a
// repository, no key does anything.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys.onList(s)
	own := keyhelp.Layer{Bindings: slices.Concat(k.FullHelp()...), Short: k.ShortHelp()}
	if !s.hasRepo || s.feed == nil {
		return []keyhelp.Layer{ui.Off(ui.MergeLayers(ctxList, own))}
	}
	list, prompting := ui.FeedLayer(s.feed)
	if prompting {
		return []keyhelp.Layer{list}
	}
	return []keyhelp.Layer{ui.MergeLayers(ctxList, own, list)}
}

// Capturing implements ui.Capturer: the list takes every key while its
// find or filter prompt is open.
func (s *Section) Capturing() bool { return s.feed != nil && s.feed.Capturing() }

// onList returns k as the list takes it: the changes that apply to the pull
// request under the cursor, and the clear key while a filter is in
// force. Back is the modal's.
func (k keyMap) onList(s *Section) keyMap {
	pr, ok := s.target()
	k = k.withChanges(s.gate(), s.mergeMethod, pr, ok)
	k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.query != "")
	k.Back.SetEnabled(false)
	k.References.SetEnabled(false)
	k.FindFile.SetEnabled(false)
	return k
}
