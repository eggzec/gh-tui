package foldtree

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Styles holds the styles of a tree.
type Styles struct {
	// Cursor styles the CursorGlyph in column 0 of the cursor row while
	// the tree is focused, and BlurredCursor while it is not.
	Cursor, BlurredCursor lipgloss.Style
	// CursorGlyph marks the cursor row, cut or padded to one cell. The
	// default is "▌".
	CursorGlyph string
	// CursorRow styles the cursor row of a focused tree. Only its
	// background is used, across the full width; it never reverses.
	CursorRow lipgloss.Style
	// Pinned styles the lines of the ancestors pinned above a scrolled tree.
	// Only its background is used, across the full width; the default is a
	// little darker than the pane and unlike the cursor row.
	Pinned lipgloss.Style
	// Guide styles the guides that hang children off their parent.
	Guide lipgloss.Style
	// Branch draws a child with siblings after it, Last the last child,
	// Line the guide that goes on past a row, and Gap its place when it
	// does not. Each is cut or padded to 3 cells. The defaults are "├─ ",
	// "╰─ ", "│  " and three spaces.
	Branch, Last, Line, Gap string
	// Arrow styles the OpenGlyph and ClosedGlyph before the text of a fold
	// row, each cut or padded to one cell. The defaults are "▾" and "▸".
	Arrow                  lipgloss.Style
	OpenGlyph, ClosedGlyph string
	// Marker styles the marker of a closed fold, at the end of its text.
	Marker lipgloss.Style
	// Ellipsis ends a text where it is cut. The default is "…".
	Ellipsis string
	// Empty styles the text shown when there are no rows, and EmptyText is
	// that text.
	Empty     lipgloss.Style
	EmptyText string
}

// DefaultStyles returns the default styles for a light or dark terminal.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#61afef"))
	dim := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#5c6370"))
	rowBG := ld(lipgloss.Color("#e4e8f0"), lipgloss.Color("#2a2f3a"))
	pinBG := ld(lipgloss.Color("#eff1f6"), lipgloss.Color("#1b1f27"))
	return Styles{
		Cursor:        lipgloss.NewStyle().Foreground(accent).Bold(true),
		BlurredCursor: lipgloss.NewStyle().Foreground(dim),
		CursorGlyph:   "▌",
		CursorRow:     lipgloss.NewStyle().Background(rowBG),
		Pinned:        lipgloss.NewStyle().Background(pinBG),
		Guide:         lipgloss.NewStyle().Foreground(dim),
		Branch:        "├─ ",
		Last:          "╰─ ",
		Line:          "│  ",
		Gap:           "   ",
		Arrow:         lipgloss.NewStyle().Foreground(dim),
		OpenGlyph:     "▾",
		ClosedGlyph:   "▸",
		Marker:        lipgloss.NewStyle().Foreground(dim),
		Ellipsis:      "…",
		Empty:         lipgloss.NewStyle().Foreground(dim),
		EmptyText:     "Nothing to show.",
	}
}

// look is what the styles draw, kept ready so that a frame renders none of
// it twice.
type look struct {
	branch, last, line, none string
	cursor, blurred          string
	open, closed             string
	bg, pin                  string
}

// pick returns a when ok, else b.
func (look) pick(ok bool, a, b string) string {
	if ok {
		return a
	}
	return b
}

func (m *Model) setStyles(s Styles) {
	m.styles = s
	cells := func(g string) string { return s.Guide.Render(termtext.Cells(g, guideWidth)) }
	m.look = look{
		branch:  cells(s.Branch),
		last:    cells(s.Last),
		line:    cells(s.Line),
		none:    termtext.Cells(s.Gap, guideWidth),
		cursor:  s.Cursor.Render(termtext.Cells(s.CursorGlyph, 1)),
		blurred: s.BlurredCursor.Render(termtext.Cells(s.CursorGlyph, 1)),
		open:    s.Arrow.Render(termtext.Cells(s.OpenGlyph, 1)),
		closed:  s.Arrow.Render(termtext.Cells(s.ClosedGlyph, 1)),
		bg:      background(s.CursorRow),
		pin:     background(s.Pinned),
	}
}

// background returns the sequence that sets the background of st, or "" when
// it has none.
func background(st lipgloss.Style) string {
	c := st.GetBackground()
	if _, none := c.(lipgloss.NoColor); none {
		return ""
	}
	return ansi.NewStyle().BackgroundColor(c).String()
}
