package pulls

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
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
	// confirm answers a question that an overlay asks.
	confirm ui.ConfirmKeys
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

		confirm: ui.NewConfirmKeys(keys),
	}
	for i, a := range [panes.Count]string{"global.pane_1", "global.pane_2", "global.pane_3", "global.pane_4", "global.pane_5", "global.pane_6"} {
		k.pane[i] = modal.Binding(a, panes.Pane(i+1).Name())
	}
	k.jump = ui.Jump(k.pane[:]...)
	return k
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
	bindings := slices.Concat([]key.Binding{k.next, k.prev}, k.pane[:], []key.Binding{k.zoom, k.back, k.owner})
	own := ui.ContextLayer(ctxModal, bindings, []key.Binding{k.next, k.jump, k.zoom})
	return []keyhelp.Layer{own, ui.ContextLayer(m.context(l), nil, nil)}
}
