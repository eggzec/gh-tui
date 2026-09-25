package finder

import tea "charm.land/bubbletea/v2"

// ChosenMsg reports that the user picked Item in the finder with ID.
type ChosenMsg struct {
	ID   int64
	Item Item
}

// CancelMsg reports that the user closed the finder with ID.
type CancelMsg struct {
	ID int64
}

// loadedMsg carries the paths that Load returned, prepared for matching.
type loadedMsg struct {
	id     int64
	corpus *corpus
	note   string
	err    error
}

// matchMsg carries the result of query seq, matched in a command.
type matchMsg struct {
	id  int64
	seq int
	res *result
}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
