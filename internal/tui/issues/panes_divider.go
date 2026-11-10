package issues

import (
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// SetDividers gives the section the dividers the user moved, which it shares
// with the other views. With none the modal's dividers stay where they start.
func (s *Section) SetDividers(d *ui.Dividers) { s.dividers = d }

// divider is the divider the modal shows now, with its default position and
// the bounds of it: the side column beside the main one of the wide layout, or
// the list beside the preview of a zoomed pane. ok is false when none shows.
func (m *detailModal) divider() (s *panes.Size, def, lo, hi int, ok bool) {
	d := m.dividers
	if d == nil {
		return nil, 0, 0, 0, false
	}
	w := m.width
	lo, hi = panes.MinWidth, w-panes.MinWidth
	if hi < lo {
		return nil, 0, 0, 0, false
	}
	l := m.paneLayout()
	if l.Zoomed {
		if _, split := l.Box(l.Focus, panes.List); split {
			return &d.List, panes.DefaultListWidth, lo, hi, true
		}
		return nil, 0, 0, 0, false
	}
	return &d.Side, w - sideWidth, lo, hi, true
}

// canDivide reports whether the modal shows a divider that a key moves. A
// question, an overlay or the links take the keys before the panes do.
func (m *detailModal) canDivide() bool {
	if m.ask != nil || m.over != nil || m.refs != nil {
		return false
	}
	_, _, _, _, ok := m.divider()
	return ok
}

// moveDivider moves the divider that shows by steps presses.
func (m *detailModal) moveDivider(steps int) {
	if s, def, lo, hi, ok := m.divider(); ok {
		m.dividers.Move(s, steps, def, lo, hi)
	}
}

// stack is the divider between the Description and the Conversation, with its
// default height of the Description and the bounds of it. ok is false when the
// wide layout does not show them both, or the focus is on another pane.
func (m *detailModal) stack() (s *panes.Size, def, lo, hi int, ok bool) {
	if m.dividers == nil {
		return nil, 0, 0, 0, false
	}
	if f := m.nav.Focus(); f != paneDescription && f != paneConversation {
		return nil, 0, 0, 0, false
	}
	if m.paneLayout().Zoomed {
		return nil, 0, 0, 0, false
	}
	h := max(m.height-m.headerRows(), 0)
	lo, hi = panes.MinHeight, h-panes.MinHeight
	if hi < lo {
		return nil, 0, 0, 0, false
	}
	return &m.dividers.Desc, descriptionHeight(h), lo, hi, true
}

// canStack reports whether the modal shows a stack divider that a key moves.
func (m *detailModal) canStack() bool {
	if m.ask != nil || m.over != nil || m.refs != nil {
		return false
	}
	_, _, _, _, ok := m.stack()
	return ok
}

// growPane grows the focused pane of the Description over the Conversation by
// dir presses, or shrinks it when dir is negative.
func (m *detailModal) growPane(dir int) {
	s, def, lo, hi, ok := m.stack()
	if !ok {
		return
	}
	_, sign := panes.Grow(int(m.nav.Focus())-int(paneDescription), 2)
	m.dividers.Move(s, dir*sign, def, lo, hi)
}

// settleDividers keeps the dividers that show within their bounds after a
// resize, so that presses at a limit never pile up out of sight.
func (m *detailModal) settleDividers() {
	if s, def, lo, hi, ok := m.divider(); ok {
		s.Settle(def, lo, hi)
	}
	if s, def, lo, hi, ok := m.stack(); ok {
		s.Settle(def, lo, hi)
	}
}
