package filterform

import tea "charm.land/bubbletea/v2"

// AppliedMsg reports that the user applied the form with ID.
type AppliedMsg struct {
	ID int64
	// Values holds each field's value by key.
	Values map[string]Value
	// Sort is the sort, or the zero Sort for a form without one.
	Sort Sort
	// Query is the GitHub query the fields make, with the free text the
	// user typed.
	Query string
}

// CancelMsg reports that the user closed the form with ID without applying
// it.
type CancelMsg struct {
	ID int64
}

// loadedMsg carries what a field's Loader returned for load seq.
type loadedMsg struct {
	id    int64
	field int
	seq   int
	items []Item
	err   error
}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
