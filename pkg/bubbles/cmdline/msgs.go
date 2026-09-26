package cmdline

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// SubmitMsg reports that the user sent Line from the command line with
// ID. Line has its surrounding spaces trimmed and is never empty; an empty
// line cancels instead.
type SubmitMsg struct {
	ID   int64
	Line string
}

// CancelMsg reports that the user closed the command line with ID without
// sending a line.
type CancelMsg struct {
	ID int64
}

// submit blurs the command line and reports the line, or cancels when it
// is blank.
func (m *Model) submit() tea.Cmd {
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return m.cancel()
	}
	m.remember(line)
	m.Blur()
	return send(SubmitMsg{ID: m.id, Line: line})
}

func (m *Model) cancel() tea.Cmd {
	m.Blur()
	return send(CancelMsg{ID: m.id})
}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
