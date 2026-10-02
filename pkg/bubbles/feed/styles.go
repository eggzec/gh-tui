package feed

import "charm.land/lipgloss/v2"

// gutterWidth is the width of the selection gutter left of every row.
const gutterWidth = 2

// Styles holds the styles of a feed.
type Styles struct {
	// Cursor styles the CursorGlyph that marks the selected row while the
	// feed is focused.
	Cursor lipgloss.Style
	// BlurredCursor styles the CursorGlyph while the feed is blurred.
	BlurredCursor lipgloss.Style
	// CursorGlyph marks the selected row in the gutter, cut or padded to
	// one cell. The default is "▌".
	CursorGlyph string
	// Placeholder styles rows whose chunk is being fetched again.
	Placeholder lipgloss.Style
	// Spinner styles the spinner of the loading row.
	Spinner lipgloss.Style
	// Loading styles the text of the loading row.
	Loading lipgloss.Style
	// Empty styles the text shown when there are no items.
	Empty lipgloss.Style
	// Error styles the message of the error row.
	Error lipgloss.Style
	// ErrorGlyph starts the error row. The default is "✗".
	ErrorGlyph string
	// ErrorSeparator goes between the text of an error and its hint, and
	// ErrorEllipsis ends the text where it is cut. The defaults are " · "
	// and "…".
	ErrorSeparator, ErrorEllipsis string
	// Ellipsis ends a row where it is cut, follows the "Loading" of the
	// loading row, and stands for a row whose chunk is being fetched
	// again. The default is "…".
	Ellipsis string
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

	return Styles{
		Cursor:         lipgloss.NewStyle().Foreground(accent),
		BlurredCursor:  lipgloss.NewStyle().Foreground(subtle),
		CursorGlyph:    "▌",
		Placeholder:    lipgloss.NewStyle().Foreground(subtle),
		Spinner:        lipgloss.NewStyle().Foreground(accent),
		Loading:        lipgloss.NewStyle().Foreground(muted),
		Empty:          lipgloss.NewStyle().Foreground(muted),
		Error:          lipgloss.NewStyle().Foreground(errColor),
		ErrorGlyph:     "✗",
		ErrorSeparator: " · ",
		ErrorEllipsis:  "…",
		Ellipsis:       "…",
		Hint:           lipgloss.NewStyle().Foreground(subtle),
	}
}
