package pulls

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// newOtherKeys returns the keys of the other split of each context that has a
// list with a preview: those lists, and the preview.
func newOtherKeys(keys config.Keymap) map[string]ui.OtherKeys {
	m := map[string]ui.OtherKeys{}
	for _, ctx := range []string{ctxMerge, ctxInvolved, dev.Context, ctxPreview} {
		m[ctx] = ui.NewOtherKeys(ui.Lookup(keys, ctx))
	}
	return m
}

// otherHelp returns the keys of the other split of ctx for the help: they
// work only where pressOther takes them, in a split of the focused pane.
func (m *panesModal) otherHelp(ctx string, l panes.Layout) []key.Binding {
	o := m.keys.other[ctx]
	p := m.ctl.Nav.Focus()
	_, split := l.Box(p, panes.Preview)
	on := split && p != paneFiles
	o.Down.SetEnabled(o.Down.Enabled() && on)
	o.Up.SetEnabled(o.Up.Enabled() && on)
	return []key.Binding{o.Down, o.Up}
}

// pressOther takes the key of the other split: in a zoomed pane that has a
// preview, J and K scroll the preview from the list, and move the list from
// the preview. ok is false for any other key or pane.
func (m *panesModal) pressOther(l panes.Layout, msg tea.KeyPressMsg) (cmd tea.Cmd, ok bool) {
	p := m.ctl.Nav.Focus()
	slot := m.slots.Slot(p)
	b, has := l.Box(p, panes.Preview)
	// The Files pane keeps J and K to step the files.
	if !has || slot.Preview == nil || p == paneFiles {
		return nil, false
	}
	o, has := m.keys.other[m.context(l)]
	if !has {
		return nil, false
	}
	if z := m.zoomed(l); z != nil && !m.ctl.Nav.InPreview(l) {
		// The preview is a pager, which scrolls itself.
		switch {
		case keymap.Matches(msg, o.Down):
			z.Scroll(true)
			return nil, true
		case keymap.Matches(msg, o.Up):
			z.Scroll(false)
			return nil, true
		}
	}
	return m.slots.PressOther(m, p, b, m.ctl.Nav.InPreview(l), m.keys.scroll, o, slot.Context, msg, func(press tea.KeyPressMsg) tea.Cmd {
		return m.slots.Press(m, p, press)
	})
}
