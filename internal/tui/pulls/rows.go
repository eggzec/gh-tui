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
// repository and holds the tabs.
const headerHeight = 1

// gutter lines text up with the rows of the feed, which keep two cells for
// the selection.
const gutter = "  "

// Widths of the fixed columns of a row, in cells.
const (
	stateWidth  = 2 // the state's glyph and a space
	diffWidth   = 11
	labelsWidth = 14 // the first label and how many more, such as "bug +2"
	authorWidth = 10
	// minTitle is the narrowest the title may get before columns drop. On
	// wide rows the title keeps titleShare of the width instead.
	minTitle   = 20
	titleShare = 0.4
)

// columns says which of the optional columns fit a width, and how wide the
// title is.
type columns struct {
	width int
	title int
	// ageWidth is the room of the dates, as wide as the widest.
	ageWidth                                  int
	review, checks, diff, labels, author, age bool
}

// columnsFor lays out a row of width cells, with dates of ageWidth cells
// at most. Columns drop, least important
// first, until the title has its room. The state glyphs go last, since they
// take little room and say the most.
func columnsFor(width, ageWidth int) columns {
	c := columns{width: width, ageWidth: ageWidth, review: true, checks: true, diff: true, labels: true, author: true, age: true}
	drops := []*bool{&c.diff, &c.labels, &c.author, &c.age, &c.review, &c.checks}
	want := max(minTitle, int(float64(width)*titleShare))
	for {
		c.title = width - c.fixed()
		// The title's room counts the space after the number, which
		// numbers of up to four digits leave wider.
		if c.title+1 >= want || len(drops) == 0 {
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
	n := stateWidth + ui.NumberWidth + 1
	add := func(on bool, gap, w int) {
		if on {
			n += gap + w
		}
	}
	add(c.review, 2, 1)
	add(c.checks, 1, 1)
	add(c.diff, 2, diffWidth)
	add(c.labels, 2, labelsWidth)
	add(c.author, 2, authorWidth)
	add(c.age, 2, c.ageWidth)
	return n
}

// styles are the row styles and the fragments rendered from them, made once
// per theme.
type styles struct {
	theme ui.Theme

	title, selected, author, age lipgloss.Style

	// Rows wrap text in the escape codes of these directly, which is much
	// cheaper than rendering a style for every cell of every frame.
	number, rowTitle, rowSelected paint
	rowAuthor, rowAge, diff       paint
	rowLabel                      paint

	// states are the glyphs of the states, in their colors, and badges
	// the labels of the states in the detail header.
	states, badges [ui.NumStates]string

	approved, changes, reviewRequired   string
	checksOK, checksFail, checksPending string

	repo, filterOn, filterOff, sep lipgloss.Style
	empty                          lipgloss.Style

	// The detail header and the comments.
	added, deleted, label, rule, commenter lipgloss.Style
	bar                                    string
}

// stateNames name the states of pull requests in the detail header.
var stateNames = map[ui.State]string{
	ui.PullOpen: "Open", ui.PullDraft: "Draft", ui.PullMerged: "Merged", ui.PullClosed: "Closed",
}

func newStyles(t ui.Theme, icons ui.Icons) styles {
	var states, badges [ui.NumStates]string
	for s, name := range stateNames {
		states[s] = t.State(s).Render(icons.State(s))
		badges[s] = t.State(s).Bold(true).Reverse(true).Render(" " + icons.State(s) + " " + name + " ")
	}
	return styles{
		states:         states,
		badges:         badges,
		number:         newPaint(t.Muted),
		theme:          t,
		title:          t.Text,
		selected:       t.Title,
		author:         t.Muted,
		age:            t.Subtle,
		rowTitle:       newPaint(t.Text),
		rowSelected:    newPaint(t.Title),
		rowAuthor:      newPaint(t.Muted),
		rowAge:         newPaint(t.Subtle),
		diff:           newPaint(t.Muted),
		rowLabel:       newPaint(t.Muted),
		approved:       t.Success.Render(icons.Yes),
		changes:        t.Warning.Render(icons.ChangesRequested),
		reviewRequired: t.Muted.Render(icons.ReviewRequired),
		// The glyphs of the checks screen, so the column reads as it does.
		checksOK:      t.Success.Render(icons.Run(ui.RunSuccess)),
		checksFail:    t.Error.Render(icons.Run(ui.RunFailure)),
		checksPending: t.Warning.Render(icons.Run(ui.RunQueued)),
		repo:          t.Title,
		filterOn:      t.Accent.Bold(true),
		filterOff:     t.Subtle,
		sep:           t.Subtle,
		empty:         t.Muted,
		added:         t.Success,
		deleted:       t.Error,
		label:         t.Muted,
		rule:          t.Subtle,
		commenter:     t.Title,
		bar:           t.Subtle.Render("│ "),
	}
}

// state returns the glyph of the state of pr, in its color.
func (st *styles) state(pr core.PullRequest) string {
	return st.states[ui.PullState(pr.State, pr.Draft)]
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

// noRepo is the empty state shown until a repository is picked, with hint
// saying how to pick one, wrapped to fit a narrow pane.
func (st *styles) noRepo(width, height int, hint string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	center := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	text := lipgloss.JoinVertical(lipgloss.Left,
		center.Inherit(st.selected).Render("No repository selected"),
		"",
		center.Inherit(st.empty).Render(hint),
	)
	lines := strings.Split(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text), "\n")
	lines = lines[:min(len(lines), height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "")
	}
	return strings.Join(lines, "\n")
}

// renderRow renders pr in one line of width cells.
func (s *Section) renderRow(pr core.PullRequest, selected bool, width int) string {
	if aw := s.dates.Width(); s.cols.width != width || s.cols.ageWidth != aw {
		s.cols = columnsFor(width, aw)
	}
	c, st := s.cols, &s.st

	var b strings.Builder
	b.Grow(width + 160)
	b.WriteString(st.state(pr))
	b.WriteByte(' ')
	// The number and the title link to the pull request's page.
	var link strings.Builder
	link.Grow(c.title + 64)
	num := "#" + strconv.Itoa(pr.Number)
	over := ui.NumberOver(num)
	st.number.write(&link, num)
	pad(&link, ui.NumberWidth+over-len(num)+1)

	tcells := max(c.title-over, 0)
	title, tw := truncate(pr.Title, tcells)
	ts := st.rowTitle
	if selected {
		ts = st.rowSelected
	}
	ts.write(&link, title)
	b.WriteString(s.links.Link(pr.URL, link.String()))
	pad(&b, tcells-tw)

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
	if c.labels {
		pad(&b, 2)
		st.writeLabels(&b, pr.Labels)
	}
	if c.author {
		pad(&b, 2)
		login, lw := truncate(ui.OneLine(pr.Author.Login), authorWidth)
		st.rowAuthor.write(&b, login)
		pad(&b, authorWidth-lw)
	}
	if c.age {
		pad(&b, 2)
		ago := ""
		if !pr.UpdatedAt.IsZero() {
			ago = s.dates.Short(pr.UpdatedAt, s.now())
		}
		pad(&b, c.ageWidth-ansi.StringWidth(ago))
		st.rowAge.write(&b, ago)
	}
	if over >= c.title {
		// With no title left to take from, the glyph and the number may
		// be wider than the row.
		return ansi.Truncate(b.String(), width, "")
	}
	return b.String()
}

// writeLabels writes the first of labels and how many more there are, in
// labelsWidth cells.
func (st *styles) writeLabels(b *strings.Builder, labels []core.Label) {
	if len(labels) == 0 {
		pad(b, labelsWidth)
		return
	}
	more := ""
	switch n := len(labels) - 1; {
	case n > 9:
		more = " +…"
	case n > 0:
		more = " +" + strconv.Itoa(n)
	}
	mw := utf8.RuneCountInString(more)
	name, w := truncate(labels[0].Name, labelsWidth-mw)
	st.rowLabel.write(b, name)
	st.rowAge.write(b, more)
	pad(b, labelsWidth-w-mw)
}

// truncate cuts s, text from GitHub, to at most width cells with an
// ellipsis, on one line, and returns it with its width. Most titles are
// printable ASCII, which it measures without the cost of finding grapheme
// clusters, and which needs no cleaning.
func truncate(s string, width int) (cut string, cutWidth int) {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf || s[i] < ' ' || s[i] == 0x7f {
			t := ansi.Truncate(ui.OneLine(s), width, "…")
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

// renderHeader renders the line above the list: the repository on the left
// and the tabs of the states on the right, the one shown highlighted.
func (s *Section) renderHeader() {
	if s.width <= 0 {
		s.header = ""
		return
	}
	st := &s.st
	var right strings.Builder
	for i, t := range tabs {
		if i > 0 {
			right.WriteString(st.sep.Render(" · "))
		}
		if t.state == s.tab {
			right.WriteString(st.filterOn.Render(t.label))
		} else {
			right.WriteString(st.filterOff.Render(t.label))
		}
	}
	left := gutter + st.repo.Render(s.repo.String())
	lw := ansi.StringWidth(left)
	// Narrow panes name only the tab shown, and the narrowest only the
	// repository.
	if gap := s.width - lw - ansi.StringWidth(right.String()); gap >= 2 {
		s.header = left + strings.Repeat(" ", gap) + right.String()
		return
	}
	label := tabLabel(s.tab)
	if gap := s.width - lw - len(label); gap >= 2 {
		s.header = left + strings.Repeat(" ", gap) + st.filterOn.Render(label)
		return
	}
	s.header = ansi.Truncate(left, s.width, "…")
	s.header += strings.Repeat(" ", s.width-ansi.StringWidth(s.header))
}

// emptyText is what the feed says when no pull request is in the tab, with
// the key that shows more.
func (s *Section) emptyText() string {
	kind := strings.ToLower(tabLabel(s.tab)) + " pull requests"
	if s.tab == "" {
		kind = "pull requests"
	}
	if s.query != "" {
		return ui.NoMatch(kind, ui.KeyOf(s.keys.ClearFilter))
	}
	text := ui.None(kind)
	switch next := nextTab(s.tab, 1); {
	case s.tab == "":
		return text
	case next == "":
		return ui.Press(text, ui.KeyOf(s.keys.NextTab), "show all of them")
	default:
		return ui.Press(text, ui.KeyOf(s.keys.NextTab), "show "+strings.ToLower(tabLabel(next))+" ones")
	}
}

// pullKey identifies a pull request in the feed, so a reload keeps the
// selection on it.
func pullKey(pr core.PullRequest) string {
	return strconv.Itoa(pr.Number)
}

// badge returns the state badge of pr.
func (st *styles) badge(pr core.PullRequest) string {
	return st.badges[ui.PullState(pr.State, pr.Draft)]
}

// reviewText spells out the review decision, or returns "" when reviews
// aren't required.
func (st *styles) reviewText(d core.ReviewDecision) string {
	switch d {
	case core.ReviewApproved:
		return st.approved + st.author.Render(" approved")
	case core.ReviewChangesRequested:
		return st.changes + st.author.Render(" changes requested")
	case core.ReviewRequired:
		return st.reviewRequired + st.author.Render(" review required")
	default:
		return ""
	}
}

// checksSummary counts the checks of d by outcome, or names the overall
// state when the counts aren't known. It returns "" without checks.
func (st *styles) checksSummary(d *core.PullRequestDetail) string {
	c := d.CheckCounts
	if c.Total() == 0 {
		switch d.Checks {
		case core.ChecksSuccess:
			return st.age.Render("CI  ") + st.checksOK + st.author.Render(" passed")
		case core.ChecksFailure:
			return st.age.Render("CI  ") + st.checksFail + st.author.Render(" failing")
		case core.ChecksPending:
			return st.age.Render("CI  ") + st.checksPending + st.author.Render(" pending")
		default:
			return ""
		}
	}
	parts := make([]string, 0, 3)
	if c.Failed > 0 {
		parts = append(parts, st.checksFail+st.author.Render(" "+strconv.Itoa(c.Failed)+" failing"))
	}
	if c.Pending > 0 {
		parts = append(parts, st.checksPending+st.author.Render(" "+strconv.Itoa(c.Pending)+" pending"))
	}
	if c.Passed > 0 {
		parts = append(parts, st.checksOK+st.author.Render(" "+strconv.Itoa(c.Passed)+" passed"))
	}
	return st.age.Render("CI  ") + strings.Join(parts, st.sep.Render(", "))
}
