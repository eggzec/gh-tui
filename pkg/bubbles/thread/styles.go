package thread

import (
	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/markdown"
)

// Styles holds the styles of a thread. The header and the comments are
// styled by the caller, so these cover the markdown body and the lines the
// thread draws itself.
type Styles struct {
	// Spinner styles the spinner shown while something loads.
	Spinner lipgloss.Style
	// Loading styles the text next to the spinner.
	Loading lipgloss.Style
	// Empty styles the line shown when there are no comments.
	Empty lipgloss.Style
	// Error styles the error message.
	Error lipgloss.Style
	// Key styles the pointer at the diagram the toggle key opens.
	Key lipgloss.Style
	// Hint styles the text around a key in a hint.
	Hint lipgloss.Style
	// Markdown is the glamour style of the body and of what
	// [Model.Markdown] renders. [WithMarkdownStyle] overrides it.
	Markdown ansi.StyleConfig
}

// DefaultStyles returns the default styles for a light or dark terminal.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	failure := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))

	return Styles{
		Spinner:  lipgloss.NewStyle().Foreground(accent),
		Loading:  lipgloss.NewStyle().Foreground(muted),
		Empty:    lipgloss.NewStyle().Foreground(muted),
		Error:    lipgloss.NewStyle().Foreground(failure),
		Key:      lipgloss.NewStyle().Foreground(accent).Bold(true),
		Hint:     lipgloss.NewStyle().Foreground(subtle),
		Markdown: markdown.DefaultStyle(isDark),
	}
}
