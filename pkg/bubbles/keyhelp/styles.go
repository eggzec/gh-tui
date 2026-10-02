package keyhelp

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Styles holds the styles of the help.
type Styles struct {
	// Title styles the title, and Count the number of bindings listed
	// after it.
	Title lipgloss.Style
	Count lipgloss.Style
	// Prompt styles the glyph before the query, Text what the user types,
	// Placeholder the text shown while the query is empty, and Cursor the
	// cursor, with its foreground. The cursor doesn't blink.
	Prompt      lipgloss.Style
	Text        lipgloss.Style
	Placeholder lipgloss.Style
	Cursor      lipgloss.Style
	// Capture styles the line shown while a key is captured, and the key
	// that filters the list.
	Capture lipgloss.Style
	// Key styles the keys of a binding, Desc its description and Source
	// the source of its layer.
	Key    lipgloss.Style
	Desc   lipgloss.Style
	Source lipgloss.Style
	// Disabled styles a whole row that no key reaches: a disabled or a
	// typed binding.
	Disabled lipgloss.Style
	// Conflict styles the mark and the losses of a binding that loses a
	// key within its layer, and Shadowed those of one that loses a key to
	// an earlier layer.
	Conflict lipgloss.Style
	Shadowed lipgloss.Style
	// Typed styles the line of a key that is typed in.
	Typed lipgloss.Style
	// Empty styles the text shown when nothing matches.
	Empty lipgloss.Style
	// PromptGlyph goes before the query. The default is "›".
	PromptGlyph string
	// WarnGlyph marks a binding that loses a key to another, cut or
	// padded to one cell, and LossGlyph starts the line under it that
	// says who gets the key. The defaults are "⚠" and "↳".
	WarnGlyph, LossGlyph string
	// Separator goes between the parts of a line, and Ellipsis ends a
	// text cut to its room. The defaults are " · " and "…".
	Separator, Ellipsis string
}

// DefaultStyles returns calm styles for a light or dark terminal, with the
// accent color only on the keys and the query.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	warn := ld(lipgloss.Color("#9a6700"), lipgloss.Color("#e0af68"))
	errColor := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))

	return Styles{
		Title:       lipgloss.NewStyle().Foreground(text).Bold(true),
		Count:       lipgloss.NewStyle().Foreground(subtle),
		Prompt:      lipgloss.NewStyle().Foreground(accent),
		Text:        lipgloss.NewStyle().Foreground(text),
		Placeholder: lipgloss.NewStyle().Foreground(subtle),
		Cursor:      lipgloss.NewStyle().Foreground(accent),
		Capture:     lipgloss.NewStyle().Foreground(accent).Bold(true),
		Key:         lipgloss.NewStyle().Foreground(accent),
		Desc:        lipgloss.NewStyle().Foreground(text),
		Source:      lipgloss.NewStyle().Foreground(muted),
		Disabled:    lipgloss.NewStyle().Foreground(subtle).Faint(true),
		Conflict:    lipgloss.NewStyle().Foreground(errColor),
		Shadowed:    lipgloss.NewStyle().Foreground(warn),
		Typed:       lipgloss.NewStyle().Foreground(subtle),
		Empty:       lipgloss.NewStyle().Foreground(muted),
		PromptGlyph: "›",
		WarnGlyph:   "⚠",
		LossGlyph:   "↳",
		Separator:   " · ",
		Ellipsis:    "…",
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles and passes them on to the input.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.input.SetStyles(inputStyles(s))
	m.prompt = s.Prompt.Render(s.PromptGlyph + " ")
	m.drawn = nil
	m.list()
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
