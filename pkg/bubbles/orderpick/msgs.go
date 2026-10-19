package orderpick

import tea "charm.land/bubbletea/v2"

// Choice is one chosen item: its Value and the direction it was chosen in.
type Choice struct {
	Value any
	// Desc is true when the item runs descending, which the user may have
	// flipped from the item's natural direction.
	Desc bool
}

// ChosenMsg reports that the user confirmed the picker with ID. Choices holds
// the chosen items in their order, which is empty when none are chosen.
type ChosenMsg struct {
	ID      int64
	Choices []Choice
}

// CancelMsg reports that the user closed the picker with ID.
type CancelMsg struct {
	ID int64
}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
