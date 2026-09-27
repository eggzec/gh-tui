package finder

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Glyphs of the finder.
const (
	promptGlyph = "› "
	cursorGlyph = "▌"
)

// Styles holds the styles of a finder.
type Styles struct {
	// Prompt styles the glyph before the input.
	Prompt lipgloss.Style
	// Text styles what the user types.
	Text lipgloss.Style
	// Placeholder styles the text shown while the query is empty.
	Placeholder lipgloss.Style
	// Cursor colors the input's cursor with its foreground.
	Cursor lipgloss.Style
	// Gutter marks the selected row.
	Gutter lipgloss.Style
	// Dir styles the directories of a path, and Name its file name.
	// SelectedName styles the file name of the selected row.
	Dir          lipgloss.Style
	Name         lipgloss.Style
	SelectedName lipgloss.Style
	// Match styles the characters that match the query. It replaces the
	// style of the part of the path they are in.
	Match lipgloss.Style
	// Detail styles the detail at the right edge of a row.
	Detail lipgloss.Style
	// Status styles the counts in the status line, and Note the note of
	// the listing.
	Status lipgloss.Style
	Note   lipgloss.Style
	// Spinner styles the spinner shown while the paths load or match.
	Spinner lipgloss.Style
	// Empty styles the text shown when nothing matches.
	Empty lipgloss.Style
	// Error styles a failed load.
	Error lipgloss.Style
}

// DefaultStyles returns calm styles for a light or dark terminal, with the
// accent color only on the prompt, the selection and the matches.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	warning := ld(lipgloss.Color("#9a6700"), lipgloss.Color("#e5c07b"))
	errColor := ld(lipgloss.Color("#c4314b"), lipgloss.Color("#f7768e"))
	return Styles{
		Prompt:       lipgloss.NewStyle().Foreground(accent),
		Text:         lipgloss.NewStyle().Foreground(text),
		Placeholder:  lipgloss.NewStyle().Foreground(subtle),
		Cursor:       lipgloss.NewStyle().Foreground(accent),
		Gutter:       lipgloss.NewStyle().Foreground(accent),
		Dir:          lipgloss.NewStyle().Foreground(muted),
		Name:         lipgloss.NewStyle().Foreground(text),
		SelectedName: lipgloss.NewStyle().Foreground(text).Bold(true),
		Match:        lipgloss.NewStyle().Foreground(accent).Bold(true),
		Detail:       lipgloss.NewStyle().Foreground(subtle),
		Status:       lipgloss.NewStyle().Foreground(subtle),
		Note:         lipgloss.NewStyle().Foreground(warning),
		Spinner:      lipgloss.NewStyle().Foreground(accent),
		Empty:        lipgloss.NewStyle().Foreground(muted),
		Error:        lipgloss.NewStyle().Foreground(errColor),
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.esc = newEsc(s)
	m.rows = nil
	m.spin.Style = s.Spinner
	st := textinput.StyleState{Text: s.Text, Placeholder: s.Placeholder, Prompt: s.Prompt}
	m.input.SetStyles(textinput.Styles{
		Focused: st,
		Blurred: st,
		// The cursor doesn't blink, so an idle finder never ticks.
		Cursor: textinput.CursorStyle{Color: s.Cursor.GetForeground(), Shape: tea.CursorBlock},
	})
	m.render()
}

// SetIcons sets the icons drawn before paths. See [WithIcons].
func (m *Model) SetIcons(icons Icons) {
	m.icons = icons
	m.rows = nil
	m.render()
}

// pair is the escape sequences that turn a style on and off.
type pair struct{ on, off string }

func newPair(s lipgloss.Style) pair {
	// Escape sequences never hold an x, so it marks the content.
	on, off, _ := strings.Cut(s.Render("x"), "x")
	return pair{on, off}
}

func (p pair) wrap(s string) string { return p.on + s + p.off }

// esc holds the rendered styles. Styling a run of a path by concatenating
// escape sequences is much cheaper than rendering it with lipgloss.
type esc struct {
	dir, name, selName, match pair
	detail, status, note      pair
	gutterOn, prompt          string
}

func newEsc(s Styles) esc {
	return esc{
		dir:      newPair(s.Dir),
		name:     newPair(s.Name),
		selName:  newPair(s.SelectedName),
		match:    newPair(s.Match),
		detail:   newPair(s.Detail),
		status:   newPair(s.Status),
		note:     newPair(s.Note),
		gutterOn: s.Gutter.Render(cursorGlyph) + " ",
		prompt:   s.Prompt.Render(promptGlyph),
	}
}
