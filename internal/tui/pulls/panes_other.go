package pulls

import (
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

// pressOther takes the key of the other split: in a zoomed pane that has a
// preview, J and K scroll the preview from the list, and move the list from
// the preview. ok is false for any other key or pane.
func (m *panesModal) pressOther(l panes.Layout, msg tea.KeyPressMsg) (cmd tea.Cmd, ok bool) {
	p := m.nav.Focus()
	slot := m.slots.Slot(p)
	b, has := l.Box(p, panes.Preview)
	// The Files pane keeps J and K to step the files.
	if !has || slot.Preview == nil || p == paneFiles {
		return nil, false
	}
	inPreview := m.nav.InPreview(l)
	o, has := m.keys.other[m.context(l)]
	if !has {
		return nil, false
	}
	var down bool
	switch {
	case keymap.Matches(msg, o.Down):
		down = true
	case keymap.Matches(msg, o.Up):
	default:
		return nil, false
	}
	return m.slots.ScrollOther(m, p, b, inPreview, m.keys.scroll, down, func(down bool) tea.Cmd {
		act := ".up"
		if down {
			act = ".down"
		}
		return m.slots.Press(m, p, keymap.ActionPress(slot.Context+act))
	}), true
}
