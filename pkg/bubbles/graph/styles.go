package graph

import "charm.land/lipgloss/v2"

// cursorGlyph marks the selected row in the gutter.
const cursorGlyph = "▌"

// gutterWidth is the width of the selection gutter left of every row.
const gutterWidth = 2

// Styles holds the styles of a graph.
type Styles struct {
	// Cursor marks the selected row while the graph is focused.
	Cursor lipgloss.Style
	// BlurredCursor marks the selected row while the graph is blurred.
	BlurredCursor lipgloss.Style
	// Lanes color the lanes in turn: lane i takes Lanes[i%len(Lanes)].
	Lanes []lipgloss.Style
	// Overflow styles the … that stands for the lanes not drawn.
	Overflow lipgloss.Style
	// Short styles the text before the title, such as the short SHA.
	Short lipgloss.Style
	// Title styles the title.
	Title lipgloss.Style
	// Detail styles the text after the title, such as the author.
	Detail lipgloss.Style
	// Right styles the text at the right edge, such as the age.
	Right lipgloss.Style
	// Spinner styles the spinner of the loading row.
	Spinner lipgloss.Style
	// Loading styles the text of the loading row.
	Loading lipgloss.Style
	// Empty styles the text shown when there are no commits.
	Empty lipgloss.Style
	// Error styles the message of the error row.
	Error lipgloss.Style
	// ErrorGlyph starts the error row. The default is "✗".
	ErrorGlyph string
	// Hint styles secondary text such as the retry key.
	Hint lipgloss.Style
}

// DefaultStyles returns the default styles for a light or dark terminal.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	errColor := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))

	// The accent first, then hues far enough apart to follow a lane.
	palette := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(accent),
		lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#2e8b57"), lipgloss.Color("#9ece6a"))),
		lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#9b59b6"), lipgloss.Color("#bb9af7"))),
		lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#b7791f"), lipgloss.Color("#e0af68"))),
		lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#1a8a99"), lipgloss.Color("#7dcfff"))),
		lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#c0567a"), lipgloss.Color("#f7768e"))),
	}

	return Styles{
		Cursor:        lipgloss.NewStyle().Foreground(accent),
		BlurredCursor: lipgloss.NewStyle().Foreground(subtle),
		Lanes:         palette,
		Overflow:      lipgloss.NewStyle().Foreground(subtle),
		Short:         lipgloss.NewStyle().Foreground(muted),
		Title:         lipgloss.NewStyle(),
		Detail:        lipgloss.NewStyle().Foreground(subtle),
		Right:         lipgloss.NewStyle().Foreground(subtle),
		Spinner:       lipgloss.NewStyle().Foreground(accent),
		Loading:       lipgloss.NewStyle().Foreground(muted),
		Empty:         lipgloss.NewStyle().Foreground(muted),
		Error:         lipgloss.NewStyle().Foreground(errColor),
		ErrorGlyph:    "✗",
		Hint:          lipgloss.NewStyle().Foreground(subtle),
	}
}
