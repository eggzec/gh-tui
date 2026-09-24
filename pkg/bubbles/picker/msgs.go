package picker

import tea "charm.land/bubbletea/v2"

// ChosenMsg reports that the user picked Item in the picker with ID.
type ChosenMsg struct {
	ID   int64
	Item Item
}

// CancelMsg reports that the user closed the picker with ID.
type CancelMsg struct {
	ID int64
}

// debounceMsg says the user stopped typing for query seq.
type debounceMsg struct {
	id  int64
	seq int
}

// resultMsg carries what the Search function returned for query seq.
type resultMsg struct {
	id    int64
	seq   int
	text  string
	items []Item
	err   error
}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
