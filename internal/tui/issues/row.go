package issues

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// A row is "  #123 ● Title…   bug +1   ◦ 3  octocat   3d": the number, the
// state, the title, then the labels, the comment count, the author and the
// age in fixed columns on the right.
const (
	numWidth      = 6 // "#12345"
	prefixWidth   = numWidth + 3
	commentsWidth = 5 // "◦ 999"
	authorWidth   = 8
	ageWidth      = 4 // "11mo"
	gap           = 2
	// chipName is the longest label name a chip shows.
	chipName = 11
	// moreWidth is the room for " +9" after the chips.
	moreWidth = 3
	minTitle  = 24
)

// State glyphs. Open and closed differ in shape as well as color.
const (
	openGlyph   = "●"
	closedGlyph = "✓"
	commentMark = "◦ "
	// commentMarkWidth is the width of commentMark in cells.
	commentMarkWidth = 2
)

// columns is which columns a row shows at a width, and how wide they are.
type columns struct {
	title    int
	labels   int
	chips    int
	comments bool
	author   bool
	age      bool
}

// layouts lists the columns from the fullest to the barest. A row takes the
// first that leaves the title room, so the least important columns go first:
// the second label, the author, the comments, the labels and the age.
var layouts = []columns{
	{chips: 2, comments: true, author: true, age: true},
	{chips: 1, comments: true, author: true, age: true},
	{chips: 1, comments: true, age: true},
	{chips: 1, age: true},
	{age: true},
	{},
}

// layout returns the columns of a row width cells wide.
func layout(width int) columns {
	var c columns
	for _, c = range layouts {
		if c.chips > 0 {
			c.labels = c.chips*(chipName+2) + c.chips - 1 + moreWidth
		}
		c.title = width - prefixWidth - c.right()
		if c.title >= minTitle {
			return c
		}
	}
	c.title = max(c.title, 0)
	return c
}

// right is the width of the columns right of the title, with their gaps.
func (c columns) right() int {
	w := 0
	add := func(on bool, width int) {
		if on {
			w += gap + width
		}
	}
	add(c.chips > 0, c.labels)
	add(c.comments, commentsWidth)
	add(c.author, authorWidth)
	add(c.age, ageWidth)
	return w
}

// paint wraps text in the escape codes of a style. It is built once per
// theme, so rendering a row only concatenates strings.
type paint struct{ pre, post string }

func newPaint(s lipgloss.Style) paint {
	pre, post, _ := strings.Cut(s.Render("x"), "x")
	return paint{pre, post}
}

func (p paint) write(b *strings.Builder, text string) {
	b.WriteString(p.pre)
	b.WriteString(text)
	b.WriteString(p.post)
}

// rowStyles are the styles of the list rows.
type rowStyles struct {
	number, title, selected paint
	open, closed            paint
	meta, age, more         paint
	// label is the chip of a label whose color is not valid.
	label lipgloss.Style
	dark  bool
	// The state badges of the detail header.
	openBadge, closedBadge string
}

func newRowStyles(t ui.Theme) rowStyles {
	return rowStyles{
		number:   newPaint(t.Muted),
		title:    newPaint(t.Text),
		selected: newPaint(t.Title),
		open:     newPaint(t.Success),
		closed:   newPaint(t.Subtle),
		meta:     newPaint(t.Muted),
		age:      newPaint(t.Subtle),
		more:     newPaint(t.Subtle),
		label:    t.Muted.Padding(0, 1),
		dark:     t.Dark,

		openBadge:   t.Success.Bold(true).Render(openGlyph + " Open"),
		closedBadge: t.Subtle.Bold(true).Render(closedGlyph + " Closed"),
	}
}

// renderRow renders an issue as one line of the list.
func (s *Section) renderRow(it core.Issue, selected bool, width int) string {
	c := s.cols
	if width != s.colsWidth {
		c = layout(width)
	}
	st := &s.rows
	var b strings.Builder
	b.Grow(width + 160)

	num := "#" + strconv.Itoa(it.Number)
	pad(&b, numWidth-len(num))
	st.number.write(&b, num)
	b.WriteByte(' ')
	if it.State == core.StateOpen {
		st.open.write(&b, openGlyph)
	} else {
		st.closed.write(&b, closedGlyph)
	}
	b.WriteByte(' ')

	title := st.title
	if selected {
		title = st.selected
	}
	writeFit(&b, title, clean(it.Title), c.title)

	if c.chips > 0 {
		pad(&b, gap)
		s.writeLabels(&b, it.Labels, c)
	}
	if c.comments {
		pad(&b, gap)
		if it.Comments > 0 {
			n := count(it.Comments)
			pad(&b, commentsWidth-commentMarkWidth-len(n))
			st.meta.write(&b, commentMark+n)
		} else {
			pad(&b, commentsWidth)
		}
	}
	if c.author {
		pad(&b, gap)
		writeFit(&b, st.meta, it.Author.Login, authorWidth)
	}
	if c.age {
		pad(&b, gap)
		age := ui.Ago(it.UpdatedAt, s.now())
		pad(&b, ageWidth-len(age))
		st.age.write(&b, age)
	}
	return b.String()
}

// writeLabels writes up to c.chips chips, and how many labels they leave
// out, right-aligned in the labels column.
func (s *Section) writeLabels(b *strings.Builder, labels []core.Label, c columns) {
	var chips [2]chip
	shown, w := 0, 0
	for _, l := range labels[:min(len(labels), c.chips, len(chips))] {
		ch := s.chip(l)
		cw := ch.width
		if shown > 0 {
			cw++
		}
		if w+cw > c.labels-moreWidth {
			break
		}
		chips[shown] = ch
		shown++
		w += cw
	}
	more := ""
	if n := len(labels) - shown; n > 0 {
		more = "+…"
		if n < 10 {
			more = "+" + strconv.Itoa(n)
		}
		if shown > 0 {
			w++
		}
		w += len(more)
	}
	pad(b, c.labels-w)
	for i, ch := range chips[:shown] {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(ch.text)
	}
	if more != "" {
		if shown > 0 {
			b.WriteByte(' ')
		}
		s.rows.more.write(b, more)
	}
}

// count shortens large counts, such as 1200 to "1k".
func count(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return strconv.Itoa(n/1000) + "k"
}

// writeFit writes s in p, truncated with an ellipsis or padded to width
// cells.
func writeFit(b *strings.Builder, p paint, s string, width int) {
	if width <= 0 {
		return
	}
	w, ascii := textWidth(s)
	switch {
	case w <= width:
	case ascii:
		s, w = s[:width-1]+"…", width
	default:
		s = ansi.Truncate(s, width, "…")
		w = ansi.StringWidth(s)
	}
	p.write(b, s)
	pad(b, width-w)
}

// textWidth returns the width of unstyled text, and whether it is printable
// ASCII, whose width is its length.
func textWidth(s string) (int, bool) {
	for i := range len(s) {
		if s[i] < 0x20 || s[i] >= 0x7f {
			return ansi.StringWidth(s), false
		}
	}
	return len(s), true
}

// clean keeps a title on one line.
func clean(s string) string {
	if !strings.ContainsAny(s, "\n\r\t") {
		return s
	}
	return strings.Join(strings.Fields(s), " ")
}

const spaces = "                                                                "

func pad(b *strings.Builder, n int) {
	for n > 0 {
		k := min(n, len(spaces))
		b.WriteString(spaces[:k])
		n -= k
	}
}
