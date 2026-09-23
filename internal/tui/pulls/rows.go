package pulls

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// headerHeight is the height of the line above the list that names the
// repository and the filter.
const headerHeight = 1

// gutter lines text up with the rows of the feed, which keep two cells for
// the selection.
const gutter = "  "

// Widths of the fixed columns of a row, in cells.
const (
	numberWidth = 5 // "#1234"; longer numbers push the title right
	draftWidth  = 5 // "draft"
	diffWidth   = 11
	authorWidth = 10
	ageWidth    = 4 // "11mo"
	// minTitle is the narrowest the title may get before columns drop.
	minTitle = 32
)

// columns says which of the optional columns fit a width, and how wide the
// title is.
type columns struct {
	width                                    int
	title                                    int
	draft, review, checks, diff, author, age bool
}

// columnsFor lays out a row of width cells. Columns drop, least important
// first, until the title has minTitle cells.
func columnsFor(width int) columns {
	c := columns{width: width, draft: true, review: true, checks: true, diff: true, author: true, age: true}
	drops := []*bool{&c.diff, &c.draft, &c.author, &c.age, &c.checks, &c.review}
	for {
		c.title = width - c.fixed()
		if c.title >= minTitle || len(drops) == 0 {
			break
		}
		*drops[0], drops = false, drops[1:]
	}
	c.title = max(c.title, 0)
	return c
}

// fixed returns the cells of the columns other than the title, with the
// gaps before them.
func (c columns) fixed() int {
	n := numberWidth + 1
	add := func(on bool, gap, w int) {
		if on {
			n += gap + w
		}
	}
	add(c.draft, 2, draftWidth)
	add(c.review, 2, 1)
	add(c.checks, 1, 1)
	add(c.diff, 2, diffWidth)
	add(c.author, 2, authorWidth)
	add(c.age, 2, ageWidth)
	return n
}

// styles are the row styles and the fragments rendered from them, made once
// per theme.
type styles struct {
	theme ui.Theme

	title, selected, author, age lipgloss.Style

	// Rows wrap text in the escape codes of these directly, which is much
	// cheaper than rendering a style for every cell of every frame.
	open, merged, closed, draft paint
	rowTitle, rowSelected       paint
	rowAuthor, rowAge, diff     paint

	draftMark                           string
	approved, changes, reviewRequired   string
	checksOK, checksFail, checksPending string

	repo, filterOn, filterOff, sep lipgloss.Style
	empty                          lipgloss.Style
}

func newStyles(t ui.Theme) styles {
	return styles{
		theme:          t,
		title:          t.Text,
		selected:       t.Title,
		author:         t.Muted,
		age:            t.Subtle,
		open:           newPaint(t.Success),
		merged:         newPaint(t.Accent),
		closed:         newPaint(t.Error),
		draft:          newPaint(t.Subtle),
		rowTitle:       newPaint(t.Text),
		rowSelected:    newPaint(t.Title),
		rowAuthor:      newPaint(t.Muted),
		rowAge:         newPaint(t.Subtle),
		diff:           newPaint(t.Muted),
		draftMark:      t.Subtle.Render("draft"),
		approved:       t.Success.Render("✓"),
		changes:        t.Warning.Render("±"),
		reviewRequired: t.Muted.Render("•"),
		checksOK:       t.Success.Render("●"),
		checksFail:     t.Error.Render("✗"),
		checksPending:  t.Warning.Render("○"),
		repo:           t.Title,
		filterOn:       t.Accent.Bold(true),
		filterOff:      t.Subtle,
		sep:            t.Subtle,
		empty:          t.Muted,
	}
}

// state returns the style of the state of pr: drafts are subtle whatever
// their state.
func (st *styles) state(pr core.PullRequest) paint {
	switch {
	case pr.State == core.StateMerged:
		return st.merged
	case pr.State == core.StateClosed:
		return st.closed
	case pr.Draft:
		return st.draft
	}
	return st.open
}

func (st *styles) review(d core.ReviewDecision) string {
	switch d {
	case core.ReviewApproved:
		return st.approved
	case core.ReviewChangesRequested:
		return st.changes
	case core.ReviewRequired:
		return st.reviewRequired
	default:
		return " "
	}
}

func (st *styles) checks(c core.ChecksState) string {
	switch c {
	case core.ChecksSuccess:
		return st.checksOK
	case core.ChecksFailure:
		return st.checksFail
	case core.ChecksPending:
		return st.checksPending
	default:
		return " "
	}
}

// noRepo is the empty state shown until a repository is picked.
func (st *styles) noRepo(width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		st.empty.Render("Pick a repository in Repositories (tab 4)."))
}

// renderRow renders pr in one line of width cells.
func (s *Section) renderRow(pr core.PullRequest, selected bool, width int) string {
	if s.cols.width != width {
		s.cols = columnsFor(width)
	}
	c, st := s.cols, &s.st

	var b strings.Builder
	b.Grow(width + 160)
	num := "#" + strconv.Itoa(pr.Number)
	st.state(pr).write(&b, num)
	pad(&b, numberWidth-len(num)+1)

	title, tw := truncate(pr.Title, c.title)
	ts := st.rowTitle
	if selected {
		ts = st.rowSelected
	}
	ts.write(&b, title)
	pad(&b, c.title-tw)

	if c.draft {
		pad(&b, 2)
		if pr.Draft {
			b.WriteString(st.draftMark)
		} else {
			pad(&b, draftWidth)
		}
	}
	if c.review {
		pad(&b, 2)
		b.WriteString(st.review(pr.ReviewDecision))
	}
	if c.checks {
		pad(&b, 1)
		b.WriteString(st.checks(pr.Checks))
	}
	if c.diff {
		pad(&b, 2)
		adds, dels := "+"+compact(pr.Additions), "−"+compact(pr.Deletions)
		half := (diffWidth - 1) / 2
		pad(&b, half-len(adds))
		st.diff.write(&b, adds)
		pad(&b, 1)
		st.diff.write(&b, dels)
		pad(&b, half-ansi.StringWidth(dels))
	}
	if c.author {
		pad(&b, 2)
		login, lw := truncate(pr.Author.Login, authorWidth)
		st.rowAuthor.write(&b, login)
		pad(&b, authorWidth-lw)
	}
	if c.age {
		pad(&b, 2)
		ago := ui.Ago(pr.UpdatedAt, s.now())
		pad(&b, ageWidth-len(ago))
		st.rowAge.write(&b, ago)
	}
	return b.String()
}

// truncate cuts s to at most width cells with an ellipsis, and returns it
// with its width. Most titles are ASCII, which it measures without the cost
// of finding grapheme clusters.
func truncate(s string, width int) (cut string, cutWidth int) {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf || s[i] < ' ' {
			t := ansi.Truncate(s, width, "…")
			return t, ansi.StringWidth(t)
		}
	}
	if len(s) <= width {
		return s, len(s)
	}
	if width <= 0 {
		return "", 0
	}
	return s[:width-1] + "…", width
}

// compact writes n in at most four characters, such as 12, 1.2k or 34k.
func compact(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10_000:
		return strconv.Itoa(n/1000) + "." + strconv.Itoa(n%1000/100) + "k"
	case n < 1_000_000:
		return strconv.Itoa(n/1000) + "k"
	}
	return strconv.Itoa(n/1_000_000) + "M"
}

func pad(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

// paint is the escape codes a style wraps text in, for text that needs
// nothing else from the style, such as padding or wrapping.
type paint struct {
	pre, suf string
}

func newPaint(style lipgloss.Style) paint {
	const mark = "\x00"
	pre, suf, _ := strings.Cut(style.Render(mark), mark)
	return paint{pre: pre, suf: suf}
}

func (p paint) write(b *strings.Builder, text string) {
	b.WriteString(p.pre)
	b.WriteString(text)
	b.WriteString(p.suf)
}

// filters are the states the filter cycles through, in order.
var filters = []core.State{core.StateOpen, core.StateClosed, core.StateMerged}

// renderHeader renders the line above the list: the repository on the left
// and the filters on the right, the current one highlighted.
func (s *Section) renderHeader() {
	if s.width <= 0 {
		s.header = ""
		return
	}
	st := &s.st
	var right strings.Builder
	for i, f := range filters {
		if i > 0 {
			right.WriteString(st.sep.Render(" · "))
		}
		if f == s.filter {
			right.WriteString(st.filterOn.Render(string(f)))
		} else {
			right.WriteString(st.filterOff.Render(string(f)))
		}
	}
	left := gutter + st.repo.Render(s.repo.String())
	gap := s.width - ansi.StringWidth(left) - ansi.StringWidth(right.String())
	if gap < 2 {
		s.header = ansi.Truncate(left, s.width, "…")
		s.header += strings.Repeat(" ", s.width-ansi.StringWidth(s.header))
		return
	}
	s.header = left + strings.Repeat(" ", gap) + right.String()
}

// emptyText is what the feed says when no pull request is in the filter.
func (s *Section) emptyText() string {
	text := "No " + string(s.filter) + " pull requests in " + s.repo.String() + "."
	if h := s.keys.Filter.Help(); s.keys.Filter.Enabled() {
		text += " Press " + h.Key + " to show " + string(nextFilter(s.filter)) + " ones."
	}
	return text
}

func nextFilter(f core.State) core.State {
	for i, g := range filters {
		if g == f {
			return filters[(i+1)%len(filters)]
		}
	}
	return filters[0]
}

// pullKey identifies a pull request in the feed, so a reload keeps the
// selection on it.
func pullKey(pr core.PullRequest) string {
	return strconv.Itoa(pr.Number)
}
