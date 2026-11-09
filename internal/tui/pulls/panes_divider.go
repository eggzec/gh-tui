package pulls

import (
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// SetDividers gives the section the dividers the user moved, which it shares
// with the other views. With none the modal's dividers stay where they start.
func (s *Section) SetDividers(d *ui.Dividers) { s.dividers = d }

// dividers are the dividers the modal shares, or nil.
func (m *panesModal) dividers() *ui.Dividers {
	if m.list == nil {
		return nil
	}
	return m.list.dividers
}

// divider is the divider the modal shows now, with its default position and
// the bounds of it: the side column beside the main one of the wide layout, or
// the list beside the preview of a zoomed pane. ok is false when none shows.
func (m *panesModal) divider() (s *panes.Size, def, lo, hi int, ok bool) {
	d := m.dividers()
	if d == nil {
		return nil, 0, 0, 0, false
	}
	w := m.width
	lo, hi = panes.MinWidth, w-panes.MinWidth
	if hi < lo {
		return nil, 0, 0, 0, false
	}
	l := m.layout()
	if l.Zoomed {
		if _, split := l.Box(l.Focus, panes.List); split {
			return &d.List, panes.DefaultListWidth, lo, hi, true
		}
		return nil, 0, 0, 0, false
	}
	return &d.Side, w - sideWidth, lo, hi, true
}

// canDivide reports whether the modal shows a divider that a key moves. An
// overlay, or the search of the diff beside the tree, takes the keys before
// the modal does, so the modal never sees them then.
func (m *panesModal) canDivide() bool {
	if m.top() != nil || (m.filesDiffFocused() && m.files.pv.o.Typing()) {
		return false
	}
	_, _, _, _, ok := m.divider()
	return ok
}

// moveDivider moves the divider that shows by steps presses.
func (m *panesModal) moveDivider(steps int) {
	if s, def, lo, hi, ok := m.divider(); ok {
		m.dividers().Move(s, steps, def, lo, hi)
	}
}

// settleDividers keeps the divider that shows within its bounds after a
// resize, so that presses at a limit never pile up out of sight.
func (m *panesModal) settleDividers() {
	if s, def, lo, hi, ok := m.divider(); ok {
		s.Settle(def, lo, hi)
	}
}
