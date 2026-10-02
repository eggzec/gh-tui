package finder

import (
	"strings"

	"charm.land/bubbles/v2/spinner"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Styles holds the styles of a finder.
type Styles struct {
	// Prompt styles the PromptGlyph before the input.
	Prompt lipgloss.Style
	// PromptGlyph goes before the input, followed by a space. The default
	// is "›".
	PromptGlyph string
	// Text styles what the user types.
	Text lipgloss.Style
	// Placeholder styles the text shown while the query is empty.
	Placeholder lipgloss.Style
	// Cursor colors the input's cursor with its foreground.
	Cursor lipgloss.Style
	// Gutter styles the CursorGlyph that marks the selected row.
	Gutter lipgloss.Style
	// CursorGlyph marks the selected row in the gutter, cut or padded to
	// one cell. The default is "▌".
	CursorGlyph string
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
	// SpinnerFrames are the frames the spinner draws. Zero keeps the
	// default, spinner.Dot. As many frames as the default has keep the
	// spinner drawing when they change while it spins.
	SpinnerFrames spinner.Spinner
	// Empty styles the text shown when nothing matches.
	Empty lipgloss.Style
	// Error styles a failed load.
	Error lipgloss.Style
	// ErrorGlyph starts the line of a failed load. The default is "✗".
	ErrorGlyph string
	// ErrorSeparator goes between the text of an error and its hint, and
	// ErrorEllipsis ends the text where it is cut. The defaults are " · "
	// and "…".
	ErrorSeparator, ErrorEllipsis string
	// Separator goes between the parts of the status line, and Ellipsis
	// ends text where it is cut, stands for the directories cut from the
	// start of a path, and follows the "Loading" of the status line. The
	// defaults are " · " and "…".
	Separator, Ellipsis string
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
		Prompt:         lipgloss.NewStyle().Foreground(accent),
		PromptGlyph:    "›",
		Text:           lipgloss.NewStyle().Foreground(text),
		Placeholder:    lipgloss.NewStyle().Foreground(subtle),
		Cursor:         lipgloss.NewStyle().Foreground(accent),
		Gutter:         lipgloss.NewStyle().Foreground(accent),
		CursorGlyph:    "▌",
		Dir:            lipgloss.NewStyle().Foreground(muted),
		Name:           lipgloss.NewStyle().Foreground(text),
		SelectedName:   lipgloss.NewStyle().Foreground(text).Bold(true),
		Match:          lipgloss.NewStyle().Foreground(accent).Bold(true),
		Detail:         lipgloss.NewStyle().Foreground(subtle),
		Status:         lipgloss.NewStyle().Foreground(subtle),
		Note:           lipgloss.NewStyle().Foreground(warning),
		Spinner:        lipgloss.NewStyle().Foreground(accent),
		Empty:          lipgloss.NewStyle().Foreground(muted),
		Error:          lipgloss.NewStyle().Foreground(errColor),
		ErrorGlyph:     "✗",
		ErrorSeparator: " · ",
		ErrorEllipsis:  "…",
		Separator:      " · ",
		Ellipsis:       "…",
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
	m.spin.Spinner = spinner.Dot
	if len(s.SpinnerFrames.Frames) > 0 {
		m.spin.Spinner = s.SpinnerFrames
	}
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
	// promptWidth is the width of prompt in cells, and ellipsisWidth that
	// of the styles' Ellipsis.
	promptWidth, ellipsisWidth int
}

func newEsc(s Styles) esc {
	prompt := s.PromptGlyph + " "
	return esc{
		dir:           newPair(s.Dir),
		name:          newPair(s.Name),
		selName:       newPair(s.SelectedName),
		match:         newPair(s.Match),
		detail:        newPair(s.Detail),
		status:        newPair(s.Status),
		note:          newPair(s.Note),
		gutterOn:      s.Gutter.Render(termtext.Cells(s.CursorGlyph, 1)) + " ",
		prompt:        s.Prompt.Render(prompt),
		promptWidth:   ansi.StringWidth(prompt),
		ellipsisWidth: ansi.StringWidth(s.Ellipsis),
	}
}
