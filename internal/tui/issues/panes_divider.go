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

// stack is the divider that growing the focused pane of a stack moves: the one
// between the Description and the Conversation, or one of the side column's,
// with the default position of its cut, the bounds of it and the sign that
// makes the focused pane grow. ok is false in the narrow or zoomed layout, and
// when the area is too short to keep MinHeight rows in each pane.
func (m *detailModal) stack() (st stackDivider, ok bool) {
	if m.dividers == nil || m.paneLayout().Zoomed {
		return stackDivider{}, false
	}
	h := max(m.height-m.headerRows(), 0)
	f := m.nav.Focus()
	if f == paneDescription || f == paneConversation {
		_, st.sign = panes.Grow(int(f-paneDescription), 2)
		st.s, st.def, st.lo, st.hi, ok = m.descDivider(h)
		return st, ok
	}
	i, sign := panes.Grow(int(f-paneInvolved), 3)
	st.sign = sign
	st.s, st.def, st.lo, st.hi, ok = m.infoDivider(h, i)
	return st, ok
}

// stackDivider is a divider of a stack with its default cut, the bounds of it
// and the sign of a move that makes the focused pane grow.
type stackDivider struct {
	s           *panes.Size
	sign        int
	def, lo, hi int
}

// infoDivider is divider i of the side column in an area h high, with its
// default cut and the bounds of it: the cut above it and the one below it, as
// they show now, stay MinHeight rows away.
func (m *detailModal) infoDivider(h, i int) (s *panes.Size, def, lo, hi int, ok bool) {
	if m.dividers == nil || h < 3*panes.MinHeight {
		return nil, 0, 0, 0, false
	}
	first, second := infoCuts(m.dividers, h)
	if i == 0 {
		return &m.dividers.Info[0], h / 3, panes.MinHeight, second - panes.MinHeight, true
	}
	return &m.dividers.Info[1], 2 * (h / 3), first + panes.MinHeight, h - panes.MinHeight, true
}

// canStack reports whether the modal shows a stack divider that a key moves.
func (m *detailModal) canStack() bool {
	if m.ask != nil || m.over != nil || m.refs != nil {
		return false
	}
	_, ok := m.stack()
	return ok
}

// growPane grows the focused pane of a stack by dir presses, or shrinks it when
// dir is negative.
func (m *detailModal) growPane(dir int) {
	st, ok := m.stack()
	if !ok {
		return
	}
	m.dividers.Move(st.s, dir*st.sign, st.def, st.lo, st.hi)
}

// settleDividers keeps the dividers that show within their bounds after a
// resize, so that presses at a limit never pile up out of sight.
func (m *detailModal) settleDividers() {
	if s, def, lo, hi, ok := m.divider(); ok {
		s.Settle(def, lo, hi)
	}
	if m.dividers == nil || m.paneLayout().Zoomed {
		return
	}
	h := max(m.height-m.headerRows(), 0)
	if s, def, lo, hi, ok := m.descDivider(h); ok {
		s.Settle(def, lo, hi)
	}
	// The cuts settle from the top, as they are placed.
	for i := range m.dividers.Info {
		if s, def, lo, hi, ok := m.infoDivider(h, i); ok {
			s.Settle(def, lo, hi)
		}
	}
}

// descDivider is the divider between the Description and the Conversation in
// an area h high, with its default and bounds.
func (m *detailModal) descDivider(h int) (s *panes.Size, def, lo, hi int, ok bool) {
	lo, hi = panes.MinHeight, h-panes.MinHeight
	if m.dividers == nil || hi < lo {
		return nil, 0, 0, 0, false
	}
	return &m.dividers.Desc, descriptionHeight(h), lo, hi, true
}
