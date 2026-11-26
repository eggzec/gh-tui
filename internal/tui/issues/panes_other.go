package issues

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// pressOther takes the key of the other split: in a zoomed pane that has a
// preview, J and K scroll the preview from the list, and move the list from
// the preview. ok is false for any other key or pane.
func (m *detailModal) pressOther(l panes.Layout, msg tea.KeyPressMsg) (cmd tea.Cmd, ok bool) {
	p := m.nav.Focus()
	b, has := l.Box(p, panes.Preview)
	if !has || m.slots == nil || m.slots.Slot(p).Preview == nil {
		return nil, false
	}
	o := m.keys.panes.other
	var down bool
	switch {
	case keymap.Matches(msg, o.Down):
		down = true
	case keymap.Matches(msg, o.Up):
	default:
		return nil, false
	}
	ctx := ctxSideInvolved
	if p == paneDevelopment {
		ctx = dev.Context
	}
	return m.slots.ScrollOther(m, p, b, m.nav.InPreview(l), m.keys.panes.scroll, down, func(down bool) tea.Cmd {
		act := ".up"
		if down {
			act = ".down"
		}
		move, _ := m.pressSide(keymap.ActionPress(ctx + act))
		return tea.Batch(move, m.follow())
	}), true
}
