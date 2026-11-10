package actions

import (
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// SetDividers gives the modal the dividers the user moved, which it shares
// with the other views. With none the modal's divider stays where it starts.
func (m *Modal) SetDividers(d *ui.Dividers) {
	m.dividers = d
	m.layout()
}

// runsSplit is the width of the runs pane in an area w cells wide, moved by
// the user's offset when there is one.
func runsSplit(d *ui.Dividers, w int) int {
	if d == nil {
		return min(runsWidth, w)
	}
	return d.List.Of(runsWidth, panes.MinWidth, w-panes.MinWidth)
}

// divider is the divider the modal shows now, with its default position and
// the bounds of it: the runs beside the annotations. ok is false when the
// panes do not show side by side.
func (m *Modal) divider() (s *panes.Size, def, lo, hi int, ok bool) {
	if m.dividers == nil {
		return nil, 0, 0, 0, false
	}
	w, _ := m.area()
	lo, hi = panes.MinWidth, w-panes.MinWidth
	if showsOne(w, m.zoom) || hi < lo {
		return nil, 0, 0, 0, false
	}
	return &m.dividers.List, runsWidth, lo, hi, true
}

// canDivide reports whether the modal shows a divider that a key moves. A
// question or the prompt of the runs takes the keys before the panes do.
func (m *Modal) canDivide() bool {
	if m.ask != nil || m.runs.Capturing() {
		return false
	}
	_, _, _, _, ok := m.divider()
	return ok
}

// moveDivider moves the divider by steps presses and lays the panes out again.
func (m *Modal) moveDivider(steps int) {
	if s, def, lo, hi, ok := m.divider(); ok {
		m.dividers.Move(s, steps, def, lo, hi)
		m.layout()
	}
}

// resetDividers puts every divider back where it starts.
func (m *Modal) resetDividers() {
	m.dividers.Reset()
	m.layout()
}

// settleDividers keeps the divider that shows within its bounds after a
// resize, so that presses at a limit never pile up out of sight.
func (m *Modal) settleDividers() {
	if s, def, lo, hi, ok := m.divider(); ok {
		s.Settle(def, lo, hi)
	}
}
