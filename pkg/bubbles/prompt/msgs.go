package prompt

import tea "charm.land/bubbletea/v2"

// SubmitMsg reports that the user submitted the prompt with ID. Value is
// what they typed, as is; an empty value is reported too, so the parent
// decides what it means.
type SubmitMsg struct {
	ID    int64
	Value string
}

// CancelMsg reports that the user cancelled the prompt with ID.
type CancelMsg struct {
	ID int64
}

func (m Model) submit() tea.Cmd {
	msg := SubmitMsg{ID: m.id, Value: m.Value()}
	return func() tea.Msg { return msg }
}

func (m Model) cancel() tea.Cmd {
	msg := CancelMsg{ID: m.id}
	return func() tea.Msg { return msg }
}
