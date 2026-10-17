package files

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The IDs and kinds of the rows.
const (
	// IDCommits and IDFiles are the rows that title the two sections, both
	// folds.
	IDCommits = "section:commits"
	IDFiles   = "section:files"
	// KindCommit is the kind of the row of a commit.
	KindCommit = "commit"

	idRule    = "rule"
	idDivider = "divider"
	idCapped  = "capped"
	idPrefix  = "commit:"

	defaultWidth = 80
	// shaLen is how many characters of a commit's SHA the rows show.
	shaLen = 7
	// maxAuthor is the most cells the name of an author takes on the right.
	maxAuthor = 24
	// commitDepth is the level of the rows of the commits, and of what sits
	// between them.
	commitDepth = 1
)

// CommitID returns the ID of the row of the commit with the given OID.
func CommitID(oid string) string { return idPrefix + oid }

// Data is what the rows show: the commits of the pull request, oldest first,
// and the range of them that is selected.
type Data struct {
	Commits core.PullCommits
	Range   Range
}

// Options are how the rows are drawn.
type Options struct {
	// Width is the cells of the tree the rows are drawn in; 0 means 80.
	Width int
	// Styles draws the rows.
	Styles Styles
	// Dates writes the age of a commit, relative to Now.
	Dates ui.Dates
	Now   time.Time
	// Folded is whether the Commits section is folded now, since its title
	// says more of the selection while it is.
	Folded bool
}

// Short returns the first characters of a commit's SHA.
func Short(oid string) string {
	if len(oid) > shaLen {
		return oid[:shaLen]
	}
	return oid
}

// Rows returns the rows of the pane: the Commits section with a row for each
// commit, a rule, and the Files section.
func Rows(d Data, o Options) []foldtree.Row {
	b := builder{o: o, st: &o.Styles, width: o.Width}
	if b.width <= 0 {
		b.width = defaultWidth
	}
	cs := d.Commits.Commits
	rows := make([]foldtree.Row, 0, len(cs)+5)
	rows = append(rows, b.commits(d))

	before, divided := Divider(d.Commits)
	for i, c := range cs {
		if divided && i == before {
			rows = append(rows, b.divider())
		}
		rows = append(rows, b.commit(c, d.Range.Contains(i)))
	}
	if d.Commits.Capped {
		rows = append(rows, foldtree.Row{
			ID:    idCapped,
			Depth: commitDepth,
			Lines: []string{b.st.dim.Render(fmt.Sprintf("first %d commits shown", core.MaxPullCommits))},
			Skip:  true,
		})
	}

	rows = append(rows, b.rule(), foldtree.Row{
		ID:    IDFiles,
		Fold:  true,
		Lines: []string{b.st.bold.Render("Files")},
	})
	return rows
}

type builder struct {
	o     Options
	st    *Styles
	width int
}

// commits is the title of the Commits section.
func (b *builder) commits(d Data) foldtree.Row {
	n := len(d.Commits.Commits)
	title := b.st.bold.Render("Commits")
	switch {
	case n == 0:
	case b.o.Folded && d.Range.Len() == n:
		title += b.st.dim.Render(fmt.Sprintf(" all %d commits", n))
	default:
		title += b.st.dim.Render(" " + b.selection(d))
	}
	return foldtree.Row{
		ID:     IDCommits,
		Fold:   true,
		Closed: true,
		Marker: b.st.Glyphs.Fold,
		Lines:  []string{title},
	}
}

// selection says how much is selected and where it runs from and to, such as
// "3 of 12 · 1a2b3c4…5d6e7f8".
func (b *builder) selection(d Data) string {
	cs, r := d.Commits.Commits, d.Range
	if r.Len() == 0 || r.First < 0 || r.Last >= len(cs) {
		return strconv.Itoa(len(cs)) + " commits"
	}
	s := fmt.Sprintf("%d of %d %s %s", r.Len(), len(cs), b.st.Glyphs.Dot, Short(cs[r.First].OID))
	if r.Len() > 1 {
		s += b.st.Glyphs.Cut + Short(cs[r.Last].OID)
	}
	return s
}

// commit is the row of c, whose glyph and SHA are accented while it is in the
// range.
func (b *builder) commit(c core.PullCommit, in bool) foldtree.Row {
	st := b.st
	glyph, sha := st.out, st.dim.Bold(true).Render(Short(c.OID))
	subject := st.dim.Render(termtext.OneLine(c.Headline))
	if in {
		glyph, sha = st.in, st.accent.Render(Short(c.OID))
		subject = termtext.OneLine(c.Headline)
	}
	left := glyph + " " + sha + " "
	if icon := st.icon(c.Checks); icon != "" {
		left += icon + " "
	}
	left += subject

	age := ""
	if !c.CommittedAt.IsZero() {
		age = b.o.Dates.Short(c.CommittedAt, b.o.Now)
	}
	right := age
	if author := termtext.OneLine(c.Author); author != "" {
		full := termtext.Truncate(author, maxAuthor, st.Glyphs.Cut) + "  " + age
		// The author goes before the subject is cut.
		avail := b.width - 1 - 3*commitDepth
		if ansi.StringWidth(left)+1+ansi.StringWidth(full) <= avail {
			right = full
		}
	}
	return foldtree.Row{
		ID:    CommitID(c.OID),
		Depth: commitDepth,
		Kind:  KindCommit,
		Lines: []string{left},
		Right: st.dim.Render(strings.TrimSpace(right)),
	}
}

// divider marks where the commits after the viewer's last review start.
func (b *builder) divider() foldtree.Row {
	g := b.st.Glyphs.Rule
	return foldtree.Row{
		ID:    idDivider,
		Depth: commitDepth,
		Lines: []string{b.st.dim.Render(g + g + " your last review " + g + g)},
		Skip:  true,
	}
}

// rule is the line between the sections, as wide as the tree.
func (b *builder) rule() foldtree.Row {
	return foldtree.Row{
		ID:    idRule,
		Lines: []string{b.st.dim.Render(strings.Repeat(b.st.Glyphs.Rule, max(b.width-1, 1)))},
		Skip:  true,
	}
}
