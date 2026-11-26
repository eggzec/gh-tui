package issues

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// pressOther takes the key of the other split: in a zoomed pane that has a
// preview, J and K scroll the preview from the list, and move the list from
// the preview. ok is false for any other key or pane.
func (m *detailModal) pressOther(l panes.Layout, msg tea.KeyPressMsg) (cmd tea.Cmd, ok bool) {
	p := m.ctl.Nav.Focus()
	b, has := l.Box(p, panes.Preview)
	if !has || m.slots == nil || m.slots.Slot(p).Preview == nil {
		return nil, false
	}
	ctx := ctxSideInvolved
	if p == paneDevelopment {
		ctx = dev.Context
	}
	k := m.keys.panes
	return m.slots.PressOther(m, p, b, m.ctl.Nav.InPreview(l), k.scroll, k.other, ctx, msg, func(press tea.KeyPressMsg) tea.Cmd {
		// The press that reached this modal follows the hover afterwards.
		cmd, _ := m.pressSide(press)
		return cmd
	})
}
