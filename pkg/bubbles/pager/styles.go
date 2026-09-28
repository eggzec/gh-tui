package pager

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	chromastyles "github.com/alecthomas/chroma/v2/styles"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
)

// Styles holds the styles of a pager.
type Styles struct {
	// Syntax colors the tokens of highlighted content. Only its
	// foreground colors and font styles are used, so the terminal's
	// background shows through. Nil shows every file as plain text.
	Syntax *chroma.Style
	// Text styles plain text, and tokens the syntax style leaves in the
	// default color.
	Text lipgloss.Style
	// LineNumber styles the line numbers in the gutter.
	LineNumber lipgloss.Style
	// Match styles the matches of a search, and CurrentMatch the one the
	// last jump went to. They replace the syntax colors of the match.
	Match        lipgloss.Style
	CurrentMatch lipgloss.Style
	// Name styles the name of the content in the status line.
	Name lipgloss.Style
	// Status styles the position, filter and match count in the status
	// line, and the notes that aren't errors, such as an option's new
	// value.
	Status lipgloss.Style
	// Notice styles the notes on a search, such as a pattern that found
	// nothing or didn't compile.
	Notice lipgloss.Style
	// Message styles the placeholder shown instead of lines, such as
	// "Loading…".
	Message lipgloss.Style
	// Spinner styles the spinner shown while loading.
	Spinner lipgloss.Style
	// Error styles the placeholder of content that failed to load.
	Error lipgloss.Style
	// ErrorGlyph starts the text of content that failed to load. The
	// default is "✗".
	ErrorGlyph string
	// Prompt styles the "/" or "&" before the prompt, and the "-" that
	// waits for an option, and Cursor the prompt's cursor, with its
	// foreground.
	Prompt lipgloss.Style
	Cursor lipgloss.Style
}

// DefaultStyles returns calm styles for a light or dark terminal, with the
// GitHub syntax colors.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	errColor := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))
	match := ld(lipgloss.Color("#f6e7a8"), lipgloss.Color("#4a4430"))
	current := ld(lipgloss.Color("#f2c14e"), lipgloss.Color("#e0af68"))
	onCurrent := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#1a1b26"))

	syntax := "github"
	if isDark {
		syntax = "github-dark"
	}
	return Styles{
		Syntax:       chromastyles.Get(syntax),
		Text:         lipgloss.NewStyle().Foreground(text),
		LineNumber:   lipgloss.NewStyle().Foreground(subtle),
		Match:        lipgloss.NewStyle().Foreground(text).Background(match),
		CurrentMatch: lipgloss.NewStyle().Foreground(onCurrent).Background(current),
		Name:         lipgloss.NewStyle().Foreground(text).Bold(true),
		Status:       lipgloss.NewStyle().Foreground(muted),
		Notice:       lipgloss.NewStyle().Foreground(errColor),
		Message:      lipgloss.NewStyle().Foreground(muted),
		Spinner:      lipgloss.NewStyle().Foreground(accent),
		Error:        lipgloss.NewStyle().Foreground(errColor),
		ErrorGlyph:   "✗",
		Prompt:       lipgloss.NewStyle().Foreground(accent),
		Cursor:       lipgloss.NewStyle().Foreground(accent),
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles. The highlighted tokens are kept, so a new
// syntax style applies at once.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.esc = newEsc(s)
	m.spin.Style = s.Spinner
	m.renderName()
	m.prompt.SetStyles(cmdline.Styles{
		Prompt:      s.Prompt,
		Text:        s.Text,
		Placeholder: s.Status,
		Cursor:      s.Cursor,
	})
}

// pair is the escape sequences that turn a style on and off.
type pair struct{ on, off string }

func newPair(s lipgloss.Style) pair {
	// Escape sequences never hold an x, so it marks the content.
	on, off, _ := strings.Cut(s.Render("x"), "x")
	return pair{on, off}
}

func (p pair) wrap(s string) string { return p.on + s + p.off }

// esc holds the rendered styles. Styling a token by concatenating its
// escape sequences is much cheaper than rendering it with lipgloss.
type esc struct {
	text, number, match, current pair
	status, notice, prompt       pair
	// tokens holds the style of every standard token type that differs
	// from text.
	tokens map[chroma.TokenType]pair
}

func newEsc(s Styles) esc {
	e := esc{
		text:    newPair(s.Text),
		number:  newPair(s.LineNumber),
		match:   newPair(s.Match),
		current: newPair(s.CurrentMatch),
		status:  newPair(s.Status),
		notice:  newPair(s.Notice),
		prompt:  newPair(s.Prompt),
		tokens:  map[chroma.TokenType]pair{},
	}
	if s.Syntax == nil {
		return e
	}
	base := s.Syntax.Get(chroma.Text)
	for t := range chroma.StandardTypes {
		entry := s.Syntax.Get(t)
		st := s.Text
		if entry.Colour.IsSet() && entry.Colour != base.Colour {
			st = st.Foreground(rgb(entry.Colour))
		}
		st = st.Bold(entry.Bold == chroma.Yes).
			Italic(entry.Italic == chroma.Yes).
			Underline(entry.Underline == chroma.Yes)
		if p := newPair(st); p != e.text {
			e.tokens[t] = p
		}
	}
	return e
}

// token returns the style of a token type.
func (e esc) token(t chroma.TokenType) pair {
	if p, ok := e.tokens[t]; ok {
		return p
	}
	return e.text
}

func rgb(c chroma.Colour) color.Color {
	return color.RGBA{R: c.Red(), G: c.Green(), B: c.Blue(), A: 0xff}
}
