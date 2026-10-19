package pulls

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// panesKeys are the keys of the modal of a pull request that work on any of
// its panes. The modal takes them from the global keys.
type panesKeys struct {
	// next and prev move the focus between the panes, or between the list
	// and the preview of a zoomed pane. pane focuses one pane by its digit,
	// and jump stands for them in the hints.
	next, prev key.Binding
	pane       [panes.Count]key.Binding
	jump       key.Binding
	// zoom shows the focused pane alone, back closes the modal, and help
	// is the key the footer names for the help.
	zoom, back, help key.Binding
	// owner shows the page of the author, which the app's key for the owner
	// page asks of the modal.
	owner key.Binding
	// confirm answers a question that an overlay asks, and quit closes the
	// modal from under it, which takes every key.
	confirm ui.ConfirmKeys
	quit    key.Binding
	// scroll moves through the preview of a zoomed pane, and the cursor of the
	// verdicts.
	scroll ui.MoveKeys
	// desc are the keys of the description's tree.
	desc foldtree.KeyMap
	// editDesc edits the title and the description, quoteDesc quotes the
	// description in a new comment, and descEdit is the keys of the editor.
	editDesc, quoteDesc key.Binding
	descEdit            descEditKeys
	// review opens the verdicts, choose takes the one under the cursor and
	// cancel steps back out of them. composer is the keys of the summary.
	review, choose, cancel key.Binding
	composer               composer.KeyMap
	// merge asks to merge the pull request, from any pane.
	merge key.Binding
	// find opens the finder of the changed files, and finder is its keys.
	find   key.Binding
	finder finder.KeyMap
	// helpKeys is the keys of the help the modal shows inside itself.
	helpKeys keyhelp.KeyMap
	// files is the keys of the Files pane.
	files filesKeys
	// tree is the keys of the conversation's tree.
	tree foldtree.KeyMap
	// filter is the key that changes what the conversation shows, "" when
	// none is bound.
	filter string
	// convFilter opens the kinds of items to show, convSort steps the order,
	// and pick is the keys of the checklist of the kinds.
	convFilter, convSort key.Binding
	// convResolve resolves or reopens the thread under the cursor, and
	// convDelete deletes the comment or reply there.
	convResolve, convDelete key.Binding
	pick                    picker.KeyMap
	// inv is the keys of the involved pane's rows.
	inv foldtree.KeyMap
	// rerequest asks the person under the cursor for a review again.
	rerequest key.Binding
	// diff is the keys of the diff of a file.
	diff diffKeys
	// appCheck is the keys of the page of a check run of an App.
	appCheck appCheckKeys
}

// newPanesKeys makes the keys of the modal from keys.
func newPanesKeys(keys config.Keymap) panesKeys {
	modal := ui.In(keys, ctxModal)
	k := panesKeys{
		next:  modal.Binding("global.next_pane", "next pane"),
		prev:  modal.Binding("global.prev_pane", "previous pane"),
		zoom:  modal.Binding("global.zoom", "zoom"),
		back:  modal.Binding("global.dismiss", "close"),
		help:  modal.Binding("global.help", "help"),
		owner: modal.Binding("global.owner", "author"),

		review:   modal.Binding("review", "review"),
		merge:    modal.Binding("merge", "merge"),
		choose:   modal.Binding("global.select", "choose"),
		cancel:   modal.Binding("global.back", "back"),
		composer: composer.NewKeyMap(ui.Lookup(keys, "composer")),
		find:     modal.Binding("global.find_file", "find file"),
		finder:   finder.NewKeyMap(ui.Lookup(keys, "finder")),
		helpKeys: keyhelp.NewKeyMap(ui.Lookup(keys, "help")),
		files:    newFilesKeys(keys),
		tree:     foldtree.NewKeyMap(convLookup{Context: ui.In(keys, ctxConversation), keys: keys}),
		inv:      foldtree.NewKeyMap(withoutMark(ui.Lookup(keys, ctxInvolved))),

		convFilter: ui.In(keys, ctxConversation).Binding("filter", "filter"),
		convSort:   ui.In(keys, ctxConversation).Binding("sort", "sort"),
		pick:       picker.NewKeyMap(ui.Lookup(keys, "picker")),

		convResolve: ui.In(keys, ctxConversation).Binding("resolve", "resolve"),
		convDelete:  ui.In(keys, ctxConversation).Binding("delete", "delete"),

		rerequest: ui.In(keys, ctxInvolved).Binding("rerequest", "re-request"),
		diff:      newDiffKeys(keys),
		appCheck:  newAppCheckKeys(keys),

		confirm: ui.NewConfirmKeys(keys),
		quit:    modal.Binding("global.quit", "close"),
		scroll:  ui.NewMoveKeys(ui.Lookup(keys, ctxPreview)),
		desc:    foldtree.NewKeyMap(ui.Lookup(keys, ctxDescription)),

		editDesc:  ui.In(keys, ctxDescription).Binding("edit", "edit"),
		quoteDesc: ui.In(keys, ctxDescription).Binding("comment", "quote"),
		descEdit:  newDescEditKeys(keys),
	}
	for i, a := range [panes.Count]string{"global.pane_1", "global.pane_2", "global.pane_3", "global.pane_4", "global.pane_5", "global.pane_6"} {
		k.pane[i] = modal.Binding(a, panes.Pane(i+1).Name())
	}
	k.jump = ui.Jump(k.pane[:]...)
	if fk := (convLookup{Context: ui.In(keys, ctxConversation), keys: keys}).Of("filter"); len(fk) > 0 {
		k.filter = fk[0]
	}
	return k
}

// noMark is a lookup that has no key to mark rows with, which no pane of the
// modal uses.
type noMark struct{ keymap.Scoped }

func (n noMark) Of(action string) []string {
	if action == "mark" {
		return nil
	}
	return n.Scoped.Of(action)
}

// withoutMark returns look with no mark action, which the config need not have.
func withoutMark(look keymap.Lookup) keymap.Lookup {
	if s, ok := look.(keymap.Scoped); ok {
		return noMark{s}
	}
	return look
}

// digit returns the pane whose digit msg is, if it is one.
func (k panesKeys) digit(msg tea.KeyPressMsg) (panes.Pane, bool) {
	for i, b := range k.pane {
		if keymap.Matches(msg, b) {
			return panes.Pane(i + 1), true
		}
	}
	return 0, false
}

// hintKeys joins the keys that bs show in a hint, leaving out each binding
// that is disabled or has none.
func hintKeys(bs ...key.Binding) string {
	var keys []string
	for _, b := range bs {
		if b.Enabled() && len(b.Keys()) > 0 {
			keys = append(keys, b.Help().Key)
		}
	}
	return strings.Join(keys, "/")
}

// KeyContext implements ui.Hosted: the modal's context stays the one of its
// keys while an overlay has the layers.
func (m *panesModal) KeyContext() string { return ctxModal }

// KeyLayers implements ui.Keyed: the keys of the modal, and then those of the
// focused pane. An open overlay takes every key, so its layers are the only
// ones: the modal's quit key reaches it through ui.Actor.
func (m *panesModal) KeyLayers() []keyhelp.Layer {
	if o := m.top(); o != nil {
		return o.layers()
	}
	k := m.keys
	l := m.layout()
	// Pressing the digit of the focused pane does nothing, so help doesn't
	// list it.
	k.pane[m.focus-1].SetEnabled(false)
	steps := m.canStep(l)
	k.next.SetEnabled(k.next.Enabled() && steps)
	k.prev.SetEnabled(k.prev.Enabled() && steps)
	k.zoom.SetEnabled(k.zoom.Enabled() && m.canZoom())
	k.owner.SetEnabled(k.owner.Enabled() && ui.Author(m.detail.Author) != "")
	k.find.SetEnabled(k.find.Enabled() && m.canFind())
	k.merge.SetEnabled(k.merge.Enabled() && m.loaded && m.detail.State == core.StateOpen)
	bindings := slices.Concat([]key.Binding{k.next, k.prev}, k.pane[:], []key.Binding{k.zoom, k.back, k.owner, k.review, k.merge, k.find})
	own := ui.ContextLayer(ctxModal, bindings, []key.Binding{k.next, k.jump, k.zoom})
	ctx := m.context(l)
	if m.filesDiffFocused() {
		// The focused diff beside the tree takes the keys of the diff modal.
		layers := m.files.pv.o.layers()
		if m.files.pv.o.typing() {
			return layers
		}
		return []keyhelp.Layer{own, layers[len(layers)-1]}
	}
	if ctx == ctxPreview {
		var all []key.Binding
		for _, row := range k.scroll.FullHelp() {
			all = append(all, row...)
		}
		return []keyhelp.Layer{own, ui.ContextLayer(ctx, all, k.scroll.ShortHelp())}
	}
	if ctx == ctxFiles {
		return []keyhelp.Layer{own, k.files.layer()}
	}
	if ctx == ctxMerge {
		all, short := m.merge.keys.help()
		return []keyhelp.Layer{own, ui.ContextLayer(ctx, all, short)}
	}
	if ctx == ctxConversation {
		var all []key.Binding
		for _, row := range k.tree.FullHelp() {
			for _, b := range row {
				// The tree's actions that the context lacks have no key to list.
				if len(keymap.Actions(b)) > 0 {
					all = append(all, b)
				}
			}
		}
		all = append(all, k.convFilter, k.convSort, k.convResolve, k.convDelete)
		return []keyhelp.Layer{own, ui.ContextLayer(ctx, all, k.tree.ShortHelp())}
	}
	if ctx == ctxInvolved {
		return []keyhelp.Layer{own, m.involvedLayer()}
	}
	if ctx == ctxDescription {
		var all []key.Binding
		for _, row := range k.desc.FullHelp() {
			all = append(all, row...)
		}
		if m.loaded {
			k.editDesc = m.gate().Gated(k.editDesc, ui.ActDraft, &m.detail.Issue)
			k.quoteDesc = m.gate().Gated(k.quoteDesc, ui.ActComment, &m.detail.Issue)
		}
		all = append(all, k.editDesc, k.quoteDesc)
		return []keyhelp.Layer{own, ui.ContextLayer(ctx, all, append(k.desc.ShortHelp(), k.editDesc, k.quoteDesc))}
	}
	return []keyhelp.Layer{own, ui.ContextLayer(ctx, nil, nil)}
}
