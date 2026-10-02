package prompt

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Styles holds the styles of a prompt.
type Styles struct {
	// Frame surrounds a focused prompt. The default draws a bar in the
	// accent color down its left edge. Only its left and right sides are
	// drawn, on every line.
	Frame lipgloss.Style
	// BlurredFrame surrounds a blurred prompt.
	BlurredFrame lipgloss.Style
	// Title styles the title above the input.
	Title lipgloss.Style
	// Text styles what the user types.
	Text lipgloss.Style
	// Placeholder styles the text shown while the input is empty.
	Placeholder lipgloss.Style
	// Cursor colors the cursor with its foreground. The cursor doesn't
	// blink, so a prompt never ticks.
	Cursor lipgloss.Style
	// Key styles the keys in the hint under the input.
	Key lipgloss.Style
	// Hint styles the text around the keys in the hint.
	Hint lipgloss.Style
	// Separator goes between the keys of the hint, and Ellipsis ends a
	// line cut to the width. The defaults are " · " and "…".
	Separator, Ellipsis string
}

// DefaultStyles returns calm styles for a light or dark terminal: plain
// text, and color only on the edge and the cursor.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	border := ld(lipgloss.Color("#d5d9e2"), lipgloss.Color("#3a4050"))

	frame := lipgloss.NewStyle().
		Border(lipgloss.Border{Left: "┃"}, false, false, false, true).
		PaddingLeft(1)
	return Styles{
		Frame:        frame.BorderForeground(accent),
		BlurredFrame: frame.BorderForeground(border),
		Title:        lipgloss.NewStyle().Foreground(text).Bold(true),
		Text:         lipgloss.NewStyle().Foreground(text),
		Placeholder:  lipgloss.NewStyle().Foreground(subtle),
		Cursor:       lipgloss.NewStyle().Foreground(accent),
		Key:          lipgloss.NewStyle().Foreground(muted),
		Hint:         lipgloss.NewStyle().Foreground(subtle),
		Separator:    " · ",
		Ellipsis:     "…",
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles and passes them on to the input.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.focusedEdges, m.blurredEdges = newEdges(s.Frame), newEdges(s.BlurredFrame)
	m.area.SetStyles(areaStyles(s))
	m.input.SetStyles(inputStyles(s))
	m.renderHint()
	m.layout()
}

func areaStyles(s Styles) textarea.Styles {
	st := textarea.StyleState{
		Base:        lipgloss.NewStyle(),
		Text:        s.Text,
		CursorLine:  s.Text,
		Placeholder: s.Placeholder,
		EndOfBuffer: lipgloss.NewStyle(),
		Prompt:      lipgloss.NewStyle(),
		Selection:   s.Text.Reverse(true),
	}
	return textarea.Styles{
		Focused: st,
		Blurred: st,
		Cursor:  textarea.CursorStyle{Color: s.Cursor.GetForeground(), Shape: tea.CursorBlock},
	}
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
