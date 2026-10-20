package pulls

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// headerHeight is the height of the line above the list that names the
// repository and holds the tabs.
const headerHeight = 1

// headsHeight is the height of the line under it that names the columns.
const headsHeight = 1

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
	// ic draws the separators, arrows, signs and ellipses.
	ic ui.Icons

	title, selected, author, age lipgloss.Style

	// look holds the cells of a row in each of its looks.
	look [numLooks]cells

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

// The looks of a row: as it is, selected in a blurred pane, and selected in
// a focused one.
const (
	lookPlain = iota
	lookBlurred
	lookFocused
	numLooks
)

// cells are the glyphs and paints of the cells of a row in one look. Rows
// wrap text in the escape codes of these directly, which is much cheaper
// than rendering a style for every cell of every frame.
type cells struct {
	states [ui.NumStates]string

	approved, changes, reviewRequired   string
	checksOK, checksFail, checksPending string

	number, title, author, age, diff, label paint
}

func newCells(t ui.Theme, icons ui.Icons, look int) cells {
	selected := look != lookPlain
	cell := func(s lipgloss.Style) lipgloss.Style { return t.Cell(s, selected) }
	var states [ui.NumStates]string
	for s := range stateNames {
		states[s] = cell(t.State(s)).Render(icons.State(s))
	}
	return cells{
		states:         states,
		number:         newPaint(cell(t.Muted)),
		title:          newPaint(t.Cell(t.RowTitle(selected, look == lookFocused), selected)),
		author:         newPaint(cell(t.Muted)),
		age:            newPaint(cell(t.Subtle)),
		diff:           newPaint(cell(t.Muted)),
		label:          newPaint(cell(t.Muted)),
		approved:       cell(t.Success).Render(icons.Yes),
		changes:        cell(t.Warning).Render(icons.ChangesRequested),
		reviewRequired: cell(t.Muted).Render(icons.ReviewRequired),
		// The glyphs of the checks screen, so the column reads as it does.
		checksOK:      cell(t.Success).Render(icons.Run(ui.RunSuccess)),
		checksFail:    cell(t.Error).Render(icons.Run(ui.RunFailure)),
		checksPending: cell(t.Warning).Render(icons.Run(ui.RunQueued)),
	}
}

func newStyles(t ui.Theme, icons ui.Icons) styles {
	var states, badges [ui.NumStates]string
	for s, name := range stateNames {
		states[s] = t.State(s).Render(icons.State(s))
		badges[s] = t.State(s).Bold(true).Reverse(true).Render(" " + icons.State(s) + " " + name + " ")
	}
	var look [numLooks]cells
	for i := range look {
		look[i] = newCells(t, icons, i)
	}
	return styles{
		states:         states,
		badges:         badges,
		look:           look,
		theme:          t,
		ic:             icons,
		title:          t.Text,
		selected:       t.Title,
		author:         t.Muted,
		age:            t.Subtle,
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
		bar:           t.Subtle.Render(icons.Border.Left + " "),
	}
}

// state returns the glyph of the state of pr, in its color.
func (st *cells) state(pr core.PullRequest) string {
	return st.states[ui.PullState(pr.State, pr.Draft)]
}

func (st *cells) review(d core.ReviewDecision) string {
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

func (st *cells) checks(c core.ChecksState) string {
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
	if aw := s.ageRoom(); s.cols.width != width || s.cols.ageWidth != aw {
		s.cols = columnsFor(width, aw)
	}
	c := s.cols
	look := lookPlain
	switch {
	case selected && s.focused:
		look = lookFocused
	case selected:
		look = lookBlurred
	}
	st := &s.st.look[look]

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
	title, tw := truncate(pr.Title, tcells, s.st.ic.Ellipsis)
	st.title.write(&link, title)
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
		adds, dels := "+"+compact(pr.Additions), s.st.ic.Minus+compact(pr.Deletions)
		half := (diffWidth - 1) / 2
		pad(&b, half-len(adds))
		st.diff.write(&b, adds)
		pad(&b, 1)
		st.diff.write(&b, dels)
		pad(&b, half-ansi.StringWidth(dels))
	}
	if c.labels {
		pad(&b, 2)
		st.writeLabels(&b, pr.Labels, s.st.ic)
	}
	if c.author {
		pad(&b, 2)
		login, lw := truncate(ui.OneLine(pr.Author.Login), authorWidth, s.st.ic.Ellipsis)
		st.author.write(&b, login)
		pad(&b, authorWidth-lw)
	}
	if c.age {
		pad(&b, 2)
		ago := ""
		if !pr.UpdatedAt.IsZero() {
			ago = s.dates.Short(pr.UpdatedAt, s.now())
		}
		st.age.write(&b, ago)
		pad(&b, c.ageWidth-ansi.StringWidth(ago))
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
func (st *cells) writeLabels(b *strings.Builder, labels []core.Label, ic ui.Icons) {
	if len(labels) == 0 {
		pad(b, labelsWidth)
		return
	}
	more := ""
	switch n := len(labels) - 1; {
	case n > 9:
		more = " +" + termtext.Truncate(ic.Ellipsis, 1, "")
	case n > 0:
		more = " +" + strconv.Itoa(n)
	}
	mw := ansi.StringWidth(more)
	name, w := truncate(labels[0].Name, labelsWidth-mw, ic.Ellipsis)
	st.label.write(b, name)
	st.age.write(b, more)
	pad(b, labelsWidth-w-mw)
}

// truncate cuts s, text from GitHub, to at most width cells ending in
// tail, an ellipsis, on one line, and returns it with its width. Most
// titles are printable ASCII, which it measures without the cost of
// finding grapheme clusters, and which needs no cleaning.
func truncate(s string, width int, tail string) (cut string, cutWidth int) {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf || s[i] < ' ' || s[i] == 0x7f {
			t := termtext.Truncate(ui.OneLine(s), width, tail)
			return t, ansi.StringWidth(t)
		}
	}
	if len(s) <= width {
		return s, len(s)
	}
	if width <= 0 {
		return "", 0
	}
	if tw := ansi.StringWidth(tail); tw <= width {
		return s[:width-tw] + tail, width
	}
	return s[:width], width
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

// ageRoom is the width of the dates' column: the widest date, or the room of
// its title when that is wider.
func (s *Section) ageRoom() int {
	return max(s.dates.Width(), len(ui.AgeTitle))
}

// renderTitles renders the line of column titles over the rows, laid out as
// the rows are, and keeps it in titles. It stays while the list has no rows.
func (s *Section) renderTitles() {
	if s.feed == nil {
		s.titles = ""
		return
	}
	w := max(s.width-s.feed.Gutter(), 0)
	c := columnsFor(w, s.ageRoom())
	// The gaps are columns of their own, since the rows' gaps differ.
	gap := func(on bool, n int) ui.Column {
		if !on {
			return ui.Column{}
		}
		return ui.Column{Width: n}
	}
	head := ui.Header([]ui.Column{
		{Width: stateWidth},
		{Title: "#", Width: ui.NumberWidth + 1},
		{Title: "Title", Width: c.title},
		gap(c.review, 2), {Title: "R", Width: boolWidth(c.review, 1)},
		gap(c.checks, 1), {Title: "C", Width: boolWidth(c.checks, 1)},
		gap(c.diff, 2), {Title: "Diff", Width: boolWidth(c.diff, diffWidth)},
		gap(c.labels, 2), {Title: "Labels", Width: boolWidth(c.labels, labelsWidth)},
		gap(c.author, 2), {Title: "Author", Width: boolWidth(c.author, authorWidth)},
		gap(c.age, 2), {Title: ui.AgeTitle, Width: boolWidth(c.age, c.ageWidth)},
	}, 0, s.st.ic.Ellipsis)
	head = termtext.Truncate(head, w, "")
	head += strings.Repeat(" ", max(w-ansi.StringWidth(head), 0))
	s.titles = s.indent() + s.st.theme.Muted.Render(head)
}

func boolWidth(on bool, w int) int {
	if on {
		return w
	}
	return 0
}

// indent is the blank that lines the header up with the rows, which the
// feed draws a gutter before. Before the feed exists, only the cursor's.
func (s *Section) indent() string {
	if s.feed == nil {
		return gutter
	}
	return strings.Repeat(" ", s.feed.Gutter())
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
			right.WriteString(st.sep.Render(st.ic.Separator))
		}
		if t.state == s.tab {
			right.WriteString(st.filterOn.Render(t.label))
		} else {
			right.WriteString(st.filterOff.Render(t.label))
		}
	}
	left := s.indent() + st.repo.Render(s.repo.String())
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
	s.header = termtext.Truncate(left, s.width, st.ic.Ellipsis)
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
		return ui.NoMatch(kind, ui.KeyOf(s.icons, s.keys.ClearFilter))
	}
	text := ui.None(kind)
	switch next := nextTab(s.tab, 1); {
	case s.tab == "":
		return text
	case next == "":
		return ui.Press(text, ui.KeyOf(s.icons, s.keys.NextTab), "show all of them")
	default:
		return ui.Press(text, ui.KeyOf(s.icons, s.keys.NextTab), "show "+strings.ToLower(tabLabel(next))+" ones")
	}
}

// pullKey identifies a pull request in the feed, so a reload keeps the
// selection on it.
func pullKey(pr core.PullRequest) string {
	return strconv.Itoa(pr.Number)
}
