package issues

import (
	"strings"

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
}

// newPaneKeys makes the pane keys of the modal from keys.
func newPaneKeys(keys config.Keymap) paneKeys {
	modal := ui.In(keys, ctxModal)
	k := paneKeys{
		next: modal.Binding("global.next_pane", "next pane"),
		prev: modal.Binding("global.prev_pane", "previous pane"),
		zoom: modal.Binding("global.zoom", "zoom"),
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
	return append([]key.Binding{k.next, k.prev, k.zoom}, k.pane[:]...)
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

// pressPane handles the keys that move between panes and zoom, and reports
// whether msg was one.
func (m *detailModal) pressPane(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := m.keys.panes
	l := m.paneLayout()
	switch {
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
			return nil, false
		}
		m.nav.SetFocus(p)
	}
	return m.refocus(), true
}

// refocus gives the key focus to the conversation only while it is the
// focused pane, and sizes it to its box.
func (m *detailModal) refocus() tea.Cmd {
	m.layout()
	if !m.convOn || m.composing != composeNone {
		return nil
	}
	if m.nav.Focus() == paneConversation {
		m.conv.Focus()
	} else {
		m.conv.Blur()
	}
	return nil
}
