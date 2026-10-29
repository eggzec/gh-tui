package pager

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// The methods below let a host that draws its own rows, and takes its own
// keys, use the pager's search: its prompt, its regexps, its count of the
// matches and its notes. The host passes the pager its rows with
// [WithRows] or [Model.SetRows], and reads back where the matches are.

// OpenSearch opens the search prompt, empty. The host sends the keys it
// takes while [Model.Prompting] is true to [Model.UpdatePrompt], [Model.SubmitPrompt]
// or [Model.CancelPrompt].
func (m *Model) OpenSearch() tea.Cmd { return m.openPrompt(promptSearch) }

// PromptView returns the open prompt fitted to exactly width cells.
func (m Model) PromptView(width int) string { return m.fit(m.prompt.View(), width) }

// UpdatePrompt gives msg, a key or a paste, to the open prompt. A backspace
// on an empty line closes it.
func (m *Model) UpdatePrompt(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	if !m.prompt.Focused() {
		m.closePrompt()
		return nil
	}
	return cmd
}

// SubmitPrompt closes the prompt and runs the search it holds, from the top
// of the window, as [Model.SetPosition] sets it. The returned command
// searches large content in the background; its result goes to
// [Model.Forward].
func (m *Model) SubmitPrompt() tea.Cmd {
	line := m.prompt.Value()
	m.closePrompt()
	return m.searchFor(line)
}

// CancelPrompt closes the prompt and leaves the search as it was.
func (m *Model) CancelPrompt() { m.closePrompt() }

// StepMatch jumps to the next match, or the previous one for a negative d.
func (m *Model) StepMatch(d int) { m.step(d) }

// Note returns the note on the last search, such as that it found nothing,
// or "" when there is none.
func (m Model) Note() string { return m.flash }

// ClearNote drops the note on the last search.
func (m *Model) ClearNote() { m.flash, m.flashInfo = "", false }

// MatchStatus says what the search found: "searching…" while it runs, how
// many matches it found before the first jump, or which match is current. It
// is "" when there is no search.
func (m Model) MatchStatus() string {
	s := m.search
	switch {
	case s.running:
		return "searching" + m.styles.Ellipsis
	case s.query == "":
		return ""
	case s.cur < 0:
		return fmt.Sprintf("%d matches", s.total())
	default:
		return fmt.Sprintf("match %d/%d", s.cur+1, s.total())
	}
}

// Current returns the row of the current match, after a search has jumped to
// one.
func (m Model) Current() (line int, ok bool) {
	if m.search.cur < 0 {
		return 0, false
	}
	return m.search.curLine, true
}

// Ranges returns the byte ranges of the matches of the search in row line,
// and which of them is the current match, or -1. A search of the rows that a
// pattern does not match has none.
func (m Model) Ranges(line int) (ranges [][]int, cur int) {
	s := m.search
	if s.re == nil || s.invert || line < 0 || line >= m.rows.Len() {
		return nil, -1
	}
	ranges = lineMatches(s.re, m.rows.Text(line))
	if s.cur >= 0 && s.curLine == line {
		return ranges, s.curNth
	}
	return ranges, -1
}
