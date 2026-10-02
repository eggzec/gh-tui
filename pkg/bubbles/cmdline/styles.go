package cmdline

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Styles holds the styles of a command line.
type Styles struct {
	// Prompt styles the prompt before the line.
	Prompt lipgloss.Style
	// Text styles what the user types.
	Text lipgloss.Style
	// Placeholder styles the text shown while the line is empty.
	Placeholder lipgloss.Style
	// Cursor colors the cursor with its foreground. The cursor doesn't
	// blink, so a command line never ticks.
	Cursor lipgloss.Style
	// Candidate styles a candidate on the row above the line, and Detail
	// its detail.
	Candidate, Detail lipgloss.Style
	// Selected styles the candidate inserted in the line, detail and all.
	Selected lipgloss.Style
	// More styles the marks at either end of the row that say it scrolls.
	More lipgloss.Style
	// Ellipsis ends the line or the row cut to the width. The default is
	// "…".
	Ellipsis string
}

// DefaultStyles returns calm styles for a light or dark terminal: plain
// text, and the accent color on the prompt, the cursor and the selected
// candidate.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	subtle := ld(lipgloss.Color("#6b7285"), lipgloss.Color("#8a90a0"))
	onAccent := ld(lipgloss.Color("#ffffff"), lipgloss.Color("#1a1d26"))
	return Styles{
		Prompt:      lipgloss.NewStyle().Foreground(accent).Bold(true),
		Text:        lipgloss.NewStyle().Foreground(text),
		Placeholder: lipgloss.NewStyle().Foreground(subtle),
		Cursor:      lipgloss.NewStyle().Foreground(accent),
		Candidate:   lipgloss.NewStyle().Foreground(text),
		Detail:      lipgloss.NewStyle().Foreground(subtle),
		Selected:    lipgloss.NewStyle().Foreground(onAccent).Background(accent).Bold(true),
		More:        lipgloss.NewStyle().Foreground(accent).Bold(true),
		Ellipsis:    "…",
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles and passes them on to the input.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.input.SetStyles(inputStyles(s))
	m.renderPrompt()
	m.moreLeft, m.moreRight = s.More.Render("<"), s.More.Render(">")
	m.renderItems()
	m.layout()
}

func inputStyles(s Styles) textinput.Styles {
	st := textinput.StyleState{
		Text:        s.Text,
		Placeholder: s.Placeholder,
		Suggestion:  s.Placeholder,
		Prompt:      lipgloss.NewStyle(),
	}
	return textinput.Styles{
		Focused: st,
		Blurred: st,
		Cursor:  textinput.CursorStyle{Color: s.Cursor.GetForeground(), Shape: tea.CursorBlock},
	}
}
