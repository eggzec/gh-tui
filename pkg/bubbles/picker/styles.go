package picker

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Styles holds the styles of a picker.
type Styles struct {
	// Frame surrounds the picker, which floats over other views. Its
	// border and padding are taken from the picker's size; leave its width
	// and height unset.
	Frame lipgloss.Style
	// Prompt styles the glyph before the input.
	Prompt lipgloss.Style
	// Text styles what the user types.
	Text lipgloss.Style
	// Placeholder styles the text shown while the input is empty.
	Placeholder lipgloss.Style
	// Cursor colors the input's cursor with its foreground. The cursor
	// doesn't blink, so an idle picker never ticks.
	Cursor lipgloss.Style
	// Scope styles the names of the scopes, and ActiveScope the one in use.
	Scope       lipgloss.Style
	ActiveScope lipgloss.Style
	// Status styles the number of results.
	Status lipgloss.Style
	// Spinner styles the spinner shown while a search runs.
	Spinner lipgloss.Style
	// Header styles the name of a group of items.
	Header lipgloss.Style
	// Gutter marks the selected item.
	Gutter lipgloss.Style
	// Title styles an item's title, and SelectedTitle that of the selected
	// item.
	Title         lipgloss.Style
	SelectedTitle lipgloss.Style
	// Match styles the characters of a title that match the query.
	Match lipgloss.Style
	// Detail styles the text after the title.
	Detail lipgloss.Style
	// Empty styles the text shown when nothing matches.
	Empty lipgloss.Style
	// Error styles a failed search.
	Error lipgloss.Style
	// ErrorGlyph starts the line of a failed search. The default is "✗".
	ErrorGlyph string
	// ErrorSeparator goes between the text of an error and its hint, and
	// ErrorEllipsis ends the text where it is cut. The defaults are " · "
	// and "…".
	ErrorSeparator, ErrorEllipsis string
	// PromptGlyph goes before the query, and CursorGlyph marks the
	// selected row in the gutter, cut or padded to one cell. The defaults
	// are "›" and "▌".
	PromptGlyph, CursorGlyph string
	// Ellipsis ends a line cut to the width, and the default placeholder
	// and the text while searching. The default is "…".
	Ellipsis string
}

// DefaultStyles returns calm styles for a light or dark terminal, with the
// accent color only on the selection and the matches.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	border := ld(lipgloss.Color("#d5d9e2"), lipgloss.Color("#3a4050"))
	errColor := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))

	return Styles{
		Frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		Prompt:         lipgloss.NewStyle().Foreground(accent),
		Text:           lipgloss.NewStyle().Foreground(text),
		Placeholder:    lipgloss.NewStyle().Foreground(subtle),
		Cursor:         lipgloss.NewStyle().Foreground(accent),
		Scope:          lipgloss.NewStyle().Foreground(subtle),
		ActiveScope:    lipgloss.NewStyle().Foreground(accent).Bold(true),
		Status:         lipgloss.NewStyle().Foreground(subtle),
		Spinner:        lipgloss.NewStyle().Foreground(accent),
		Header:         lipgloss.NewStyle().Foreground(muted).Bold(true),
		Gutter:         lipgloss.NewStyle().Foreground(accent),
		Title:          lipgloss.NewStyle().Foreground(text),
		SelectedTitle:  lipgloss.NewStyle().Foreground(text).Bold(true),
		Match:          lipgloss.NewStyle().Foreground(accent).Bold(true),
		Detail:         lipgloss.NewStyle().Foreground(subtle),
		Empty:          lipgloss.NewStyle().Foreground(muted),
		Error:          lipgloss.NewStyle().Foreground(errColor),
		ErrorGlyph:     "✗",
		ErrorSeparator: " · ",
		ErrorEllipsis:  "…",
		PromptGlyph:    "›",
		CursorGlyph:    "▌",
		Ellipsis:       "…",
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles and passes them on to the input and spinner.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.spin.Style = s.Spinner
	m.input.SetStyles(inputStyles(s))
	m.gutterOn = s.Gutter.Render(termtext.Cells(s.CursorGlyph, 1)) + " "
	m.prompt = s.Prompt.Render(s.PromptGlyph + " ")
	if m.placeholder == "" {
		m.input.Placeholder = "Search" + s.Ellipsis
	}
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
