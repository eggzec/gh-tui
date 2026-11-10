package ui

import "github.com/eggzec/gh-tui/pkg/bubbles/panes"

// Dividers are the dividers the user moved this session, one per kind of
// view: every view of a kind moves together. The zero value is unmoved.
type Dividers struct {
	// Step is ui.divider_step, how far a press moves a divider.
	Step int
	// Files is the divider between the files column and the right one of the
	// repository screen, List the one between a list and its preview, and
	// Side the one between the main column of a modal and its side column.
	Files, List, Side panes.Size
	// Page is the divider between the two columns of the dashboard and of the
	// owner page, which move together.
	Page panes.Size
	// Stack is the divider between the pull requests and the issues of the
	// repository screen; positive is down.
	Stack panes.Size
	// Desc is the divider between the Description and the Conversation of the
	// pull request and issue modals, which share it; positive is down.
	Desc panes.Size
}

// Moved reports whether the user moved any divider.
func (d *Dividers) Moved() bool {
	return d.Files.Moved() || d.List.Moved() || d.Side.Moved() || d.Page.Moved() || d.Stack.Moved() || d.Desc.Moved()
}

// Move moves s by steps presses, then keeps both of its panes within lo and
// hi, with def the width (or height) it has unmoved.
func (d *Dividers) Move(s *panes.Size, steps, def, lo, hi int) {
	s.Move(steps * d.Step)
	s.Settle(def, lo, hi)
}

// Reset puts every divider back to its default.
func (d *Dividers) Reset() { *d = Dividers{Step: d.Step} }
