package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// SortToggle lets a list whose sort is a sort: token of its query switch the
// sort off for a look at the list in its default order, and back on, with
// the token kept in between. Applying a filter or clearing it ends the
// switch, with On.
type SortToggle struct {
	t FilterToggle[string]
}

// Off reports whether the sort is switched off.
func (s *SortToggle) Off() bool { return s.t.Off() }

// On forgets the sort that is switched off.
func (s *SortToggle) On() { s.t.On() }

// Edited returns the query with the sort that is switched off put back, for
// what edits or switches the query as a whole.
func (s *SortToggle) Edited(cur string) string { return s.t.Edited(cur) }

// Switch is the sort key's turn: it returns the query the list shows next,
// given the query in force. It returns false when there is no sort to
// switch off.
func (s *SortToggle) Switch(cur string) (string, bool) {
	return s.t.Switch(cur, DropSort(cur))
}

// DropSort returns query without its sort: token.
func DropSort(query string) string {
	return Without(query, func(t filterform.Token) bool {
		return strings.EqualFold(t.Qualifier, "sort")
	})
}

// SortOffChip words the chips of a list whose sort is switched off for a
// title: them, and the sort dim, with "off" after it.
func SortOffChip(chips, sep string) string {
	if chips != "" {
		chips += sep
	}
	return chips + OffChip("sort", sep)
}

// NoSortToSwitch is the command of a sort key with no sort to switch off: a
// toast that says so.
func NoSortToSwitch() tea.Cmd {
	return Notify(toast.Warning, "There is no sort to switch off here.")
}
