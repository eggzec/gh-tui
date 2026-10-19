package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// FilterToggle lets a list switch its filter off for a look at the list
// without it, and back on, with the filter kept in between. F is the
// way to lose a filter, so it ends the switch too. F is a type that the
// list compares, such as its query.
//
// While the filter is off, the list shows its default and the toggle holds
// the filter that was in force; the form edits that one, and applying it
// switches the filter on again.
type FilterToggle[F comparable] struct {
	kept F
	off  bool
}

// Off reports whether the filter is switched off.
func (t *FilterToggle[F]) Off() bool { return t.off }

// Kept returns the filter that is switched off, or false while it is on.
func (t *FilterToggle[F]) Kept() (F, bool) { return t.kept, t.off }

// Switch is the filter key's turn: it returns the filter the list shows
// next, given the filter in force and the list's default. It returns false
// when there is nothing to switch: the filter is the default and none is
// kept.
func (t *FilterToggle[F]) Switch(cur, def F) (next F, ok bool) {
	if t.off {
		t.off = false
		return t.kept, true
	}
	if cur == def {
		return cur, false
	}
	t.kept, t.off = cur, true
	return def, true
}

// On forgets the filter that is switched off, for when the user applies a
// filter or clears it.
func (t *FilterToggle[F]) On() {
	var zero F
	t.kept, t.off = zero, false
}

// Edited returns the filter the form opens on: the one switched off, if
// there is one, and cur otherwise.
func (t *FilterToggle[F]) Edited(cur F) F {
	if t.off {
		return t.kept
	}
	return cur
}

// OffChip words the chips of a filter that is switched off for a title:
// them, dim, with "off" after them.
func OffChip(chips, sep string) string {
	if chips != "" {
		chips += sep
	}
	return ansi.SGR(ansi.AttrFaint) + chips + "off" + ansi.SGR(ansi.AttrNormalIntensity)
}

// NothingToSwitch is the command of a filter key with no filter to switch:
// a toast that says so.
func NothingToSwitch() tea.Cmd {
	return Notify(toast.Warning, "There is no filter to switch off here.")
}
