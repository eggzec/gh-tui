package issues

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// A row is "● #123   Title…   bug +1   ◦ 3  octocat   3d": the glyph of
// the state, the number, the title, then the labels, the comment count, the
// author and the age in fixed columns on the right. It starts the way a row
// of pull requests does, so the two lists line up.
const (
	// prefixWidth is the glyph and the number, each with a space after it.
	prefixWidth   = 2 + ui.NumberWidth + 1
	commentsWidth = 5 // the mark, a space and "999"
	authorWidth   = 8
	gap           = 2
	// chipName is the longest label name a chip shows.
	chipName = 11
	// moreWidth is the room for " +9" after the chips.
	moreWidth = 3
	// minTitle is the narrowest the title may get before columns drop. On
	// wide rows the title keeps titleShare of the width instead.
	minTitle   = 18
	titleShare = 0.35
)

// commentMarkWidth is the width of the mark of the comment count and the
// space after it, in cells.
const commentMarkWidth = 2

// columns is which columns a row shows at a width, and how wide they are.
type columns struct {
	title  int
	labels int
	// ageWidth is the room of the dates, as wide as the widest.
	ageWidth int
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

// layout returns the columns of a row width cells wide, with room for
// labels if labeled, and dates of ageWidth cells at most. The number, the
// state and the title always show.
func layout(width int, labeled bool, ageWidth int) columns {
	want := max(minTitle, int(float64(width)*titleShare))
	var c columns
	for _, c = range layouts {
		if !labeled {
			c.chips = 0
		}
		c.ageWidth = ageWidth
		if c.chips > 0 {
			c.labels = c.chips*(chipName+2) + c.chips - 1 + moreWidth
		}
		c.title = width - prefixWidth - c.right()
		if c.title >= want {
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
	add(c.age, c.ageWidth)
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
	meta, age, more         paint
	// states are the glyphs of the states of issues in their colors, and
	// badges the states in the detail header.
	states, badges [ui.NumStates]string
	// label is the chip of a label whose color is not valid.
	label lipgloss.Style
	dark  bool
	// comment marks the count of comments, with a space after it, and
	// ellipsis ends cut text.
	comment, ellipsis string
}

// stateNames name the states of issues in the detail header.
var stateNames = map[ui.State]string{
	ui.IssueOpen: "Open", ui.IssueClosed: "Closed", ui.IssueNotPlanned: "Closed as not planned",
}

func newRowStyles(t ui.Theme, icons ui.Icons) rowStyles {
	var states, badges [ui.NumStates]string
	for s, name := range stateNames {
		states[s] = t.State(s).Render(icons.State(s))
		badges[s] = t.State(s).Bold(true).Render(icons.State(s) + " " + name)
	}
	return rowStyles{
		states:   states,
		badges:   badges,
		number:   newPaint(t.Muted),
		title:    newPaint(t.Text),
		selected: newPaint(t.Title),
		meta:     newPaint(t.Muted),
		age:      newPaint(t.Subtle),
		more:     newPaint(t.Subtle),
		label:    t.Muted.Padding(0, 1),
		dark:     t.Dark,
		comment:  termtext.Cells(icons.Comment, 1) + " ",
		ellipsis: icons.Ellipsis,
	}
}

// renderRow renders an issue as one line of the list.
func (s *Section) renderRow(it core.Issue, selected bool, width int) string {
	c := s.cols
	if width != s.colsWidth || c.ageWidth != s.dates.Width() {
		c = layout(width, s.labeled, s.dates.Width())
	}
	st := &s.rows
	var b strings.Builder
	b.Grow(width + 160)

	b.WriteString(st.states[ui.IssueState(it)])
	b.WriteByte(' ')
	// The number and the title link to the issue's page.
	var link strings.Builder
	link.Grow(c.title + 64)
	num := "#" + strconv.Itoa(it.Number)
	over := ui.NumberOver(num)
	st.number.write(&link, num)
	pad(&link, ui.NumberWidth+over-len(num)+1)

	title := st.title
	if selected {
		title = st.selected
	}
	tcells := max(c.title-over, 0)
	tw := writeCut(&link, title, ui.OneLine(it.Title), tcells, st.ellipsis)
	b.WriteString(s.links.Link(it.URL, link.String()))
	pad(&b, tcells-tw)

	if c.chips > 0 {
		pad(&b, gap)
		s.writeLabels(&b, it.Labels, c)
	}
	if c.comments {
		pad(&b, gap)
		if it.Comments > 0 {
			n := count(it.Comments)
			pad(&b, commentsWidth-commentMarkWidth-len(n))
			st.meta.write(&b, st.comment+n)
		} else {
			pad(&b, commentsWidth)
		}
	}
	if c.author {
		pad(&b, gap)
		writeFit(&b, st.meta, ui.OneLine(it.Author.Login), authorWidth, st.ellipsis)
	}
	if c.age {
		pad(&b, gap)
		age := ""
		if !it.UpdatedAt.IsZero() {
			age = s.dates.Short(it.UpdatedAt, s.now())
		}
		pad(&b, c.ageWidth-ansi.StringWidth(age))
		st.age.write(&b, age)
	}
	if over >= c.title {
		// With no title left to take from, the glyph and the number may
		// be wider than the row.
		return ansi.Truncate(b.String(), width, "")
	}
	return b.String()
}

// writeLabels writes up to c.chips chips, and how many labels they leave
// out, right-aligned in the labels column.
func (s *Section) writeLabels(b *strings.Builder, labels []core.Label, c columns) {
	var chips [2]chip
	shown, w := 0, 0
	for _, l := range labels[:min(len(labels), c.chips, len(chips))] {
		ch := s.chips.get(l)
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
		more = "+" + termtext.Truncate(s.rows.ellipsis, moreWidth-1, "")
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

// writeFit writes s in p, truncated to end in tail, an ellipsis, or padded
// to width cells.
func writeFit(b *strings.Builder, p paint, s string, width int, tail string) {
	pad(b, width-writeCut(b, p, s, width, tail))
}

// writeCut writes s in p, truncated to end in tail, an ellipsis, at most
// width cells, and returns its width.
func writeCut(b *strings.Builder, p paint, s string, width int, tail string) int {
	if width <= 0 {
		return 0
	}
	w, ascii := textWidth(s)
	tw, tailASCII := textWidth(tail)
	switch {
	case w <= width:
	case ascii && tailASCII && tw <= width:
		s, w = s[:width-tw]+tail, width
	default:
		s = termtext.Truncate(s, width, tail)
		w = ansi.StringWidth(s)
	}
	p.write(b, s)
	return w
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

const spaces = "                                                                "

func pad(b *strings.Builder, n int) {
	for n > 0 {
		k := min(n, len(spaces))
		b.WriteString(spaces[:k])
		n -= k
	}
}
