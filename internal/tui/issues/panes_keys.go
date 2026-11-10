package issues

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// paneKeys are the keys of the modal that work on any of its panes. The modal
// takes them from the global keys.
type paneKeys struct {
	// next and prev move the focus between the panes. pane focuses one pane by
	// its digit, and jump stands for them in the hints.
	next, prev key.Binding
	pane       [paneCount]key.Binding
	jump       key.Binding
	// zoom shows the focused pane alone.
	zoom key.Binding
	// dividerLeft and dividerRight move the divider the modal shows, and
	// dividerReset puts every divider back.
	dividerLeft, dividerRight, dividerReset key.Binding
	// dividerUp and dividerDown grow and shrink the focused pane of the
	// Description over the Conversation.
	dividerUp, dividerDown key.Binding
}

// newPaneKeys makes the pane keys of the modal from keys.
func newPaneKeys(keys config.Keymap) paneKeys {
	modal := ui.In(keys, ctxModal)
	k := paneKeys{
		next: modal.Binding("global.next_pane", "next pane"),
		prev: modal.Binding("global.prev_pane", "previous pane"),
		zoom: modal.Binding("global.zoom", "zoom"),

		dividerLeft:  modal.Binding(config.ActionDividerLeft, "divider left"),
		dividerRight: modal.Binding(config.ActionDividerRight, "divider right"),
		dividerReset: modal.Binding(config.ActionDividerReset, "reset dividers"),
		dividerUp:    modal.Binding(config.ActionDividerUp, "grow pane"),
		dividerDown:  modal.Binding(config.ActionDividerDown, "shrink pane"),
	}
	for i, a := range [paneCount]string{"global.pane_1", "global.pane_2", "global.pane_3", "global.pane_4", "global.pane_5"} {
		k.pane[i] = modal.Binding(a, paneName(panes.Pane(i+1)))
	}
	k.jump = ui.Jump(k.pane[:]...)
	return k
}

// bindings lists every pane key once, for the help. The jump stands for the
// digits in the hints only, so the help lists the digits themselves.
func (k paneKeys) bindings() []key.Binding {
	return append([]key.Binding{k.next, k.prev, k.zoom, k.dividerLeft, k.dividerRight, k.dividerUp, k.dividerDown, k.dividerReset}, k.pane[:]...)
}

// digit returns the pane whose digit msg is, if it is one.
func (k paneKeys) digit(msg tea.KeyPressMsg) (panes.Pane, bool) {
	for i, b := range k.pane {
		if keymap.Matches(msg, b) {
			return panes.Pane(i + 1), true
		}
	}
	return 0, false
}

// pressPane handles the keys that move between panes and zoom, and reports
// whether msg was one.
func (m *detailModal) pressPane(msg tea.KeyPressMsg) bool {
	k := m.keys.panes
	l := m.paneLayout()
	switch {
	case keymap.Matches(msg, k.dividerLeft) && m.canDivide():
		m.moveDivider(-1)
	case keymap.Matches(msg, k.dividerRight) && m.canDivide():
		m.moveDivider(1)
	case keymap.Matches(msg, k.dividerUp) && m.canStack():
		m.growPane(1)
	case keymap.Matches(msg, k.dividerDown) && m.canStack():
		m.growPane(-1)
	case keymap.Matches(msg, k.dividerReset) && m.canDivide() && m.dividers.Moved():
		m.dividers.Reset()
	case keymap.Matches(msg, k.next):
		m.nav.Step(l, 1)
	case keymap.Matches(msg, k.prev):
		m.nav.Step(l, -1)
	case keymap.Matches(msg, k.zoom):
		m.zoom.Toggle()
		m.nav.Reset()
	default:
		p, ok := k.digit(msg)
		if !ok {
			return false
		}
		m.nav.SetFocus(p)
	}
	m.refocus()
	return true
}

// refocus sizes the panes and syncs the conversation's focus.
func (m *detailModal) refocus() {
	m.layout()
	m.syncConvFocus()
}

// syncConvFocus gives the key focus to the conversation only while it is the
// focused pane and no composer is open. A drawn conversation follows it, so
// a rebuilt one never stays blurred under the focus.
func (m *detailModal) syncConvFocus() {
	if !m.convOn {
		return
	}
	if m.nav.Focus() == paneConversation && m.composerOpen() == nil {
		m.conv.Focus()
	} else {
		m.conv.Blur()
	}
}
