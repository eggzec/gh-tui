package ui

import "github.com/eggzec/gh-tui/pkg/bubbles/panes"

// Dividers are the dividers the user moved this session, one per kind of
// view: every view of a kind moves together. The zero value is unmoved.
type Dividers struct {
	// Step is ui.divider_step, how far a press moves a divider.
	Step int
	// Files is the divider between the files column and the right one of the
	// repository screen, and List the one between a list and its preview.
	Files, List panes.Size
}

// Moved reports whether the user moved any divider.
func (d *Dividers) Moved() bool {
	return d.Files.Moved() || d.List.Moved()
}

// Reset puts every divider back to its default.
func (d *Dividers) Reset() { *d = Dividers{Step: d.Step} }
