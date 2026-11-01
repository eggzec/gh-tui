package treeview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// GuideMode is how a tree draws the levels of its rows.
type GuideMode int

const (
	// Connectors hangs children off their parent with ├─, ╰─ and │, three
	// cells per level.
	Connectors GuideMode = iota
	// Indent draws [Styles.IndentWidth] blank cells per level, and ignores
	// Branch, Last, Line and Gap.
	Indent
)

// Styles holds the styles of a tree.
type Styles struct {
	// Guides picks how levels are drawn. The default is [Connectors].
	Guides GuideMode
	// IndentWidth is the cells of a level in [Indent] mode; the default is 2.
	IndentWidth int
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
	// Right styles the right-edge part of a row when the row's own text
	// does not.
	Right lipgloss.Style
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
		Right:         lipgloss.NewStyle().Foreground(dim),
		IndentWidth:   defaultIndent,
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
	// level is the cells of one level of guides.
	level int
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
	m.look = newLook(s)
}

func newLook(s Styles) look {
	l := look{
		cursor:  s.Cursor.Render(termtext.Cells(s.CursorGlyph, 1)),
		blurred: s.BlurredCursor.Render(termtext.Cells(s.CursorGlyph, 1)),
		open:    s.Arrow.Render(termtext.Cells(s.OpenGlyph, 1)),
		closed:  s.Arrow.Render(termtext.Cells(s.ClosedGlyph, 1)),
		bg:      background(s.CursorRow),
		pin:     background(s.Pinned),
		level:   guideWidth,
	}
	if s.Guides == Indent {
		l.level = s.IndentWidth
		if l.level <= 0 {
			l.level = defaultIndent
		}
		blank := strings.Repeat(" ", l.level)
		l.branch, l.last, l.line, l.none = blank, blank, blank, blank
		return l
	}
	cells := func(g string) string { return s.Guide.Render(termtext.Cells(g, guideWidth)) }
	l.branch, l.last, l.line = cells(s.Branch), cells(s.Last), cells(s.Line)
	l.none = termtext.Cells(s.Gap, guideWidth)
	return l
}

// Prefix returns the guides of one row at depth, as a tree draws them: the
// guide of each level above, from more (more[k] is whether a row of level k
// follows) and the connector of the row's own level, a branch unless last.
// It is for hand-drawn lists that share the look of a tree. Depth 0 has no
// guides.
func Prefix(s Styles, depth int, last bool, more []bool) string {
	if depth <= 0 {
		return ""
	}
	l := newLook(s)
	var b strings.Builder
	for k := 1; k < depth; k++ {
		b.WriteString(l.pick(k < len(more) && more[k], l.line, l.none))
	}
	b.WriteString(l.pick(last, l.last, l.branch))
	return b.String()
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
