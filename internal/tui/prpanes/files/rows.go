package files

import (
	"fmt"
	"slices"
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
	// KindFile is the kind of the row of a file.
	KindFile = "file"

	idRule      = "rule"
	idDivider   = "divider"
	idCapped    = "capped"
	idFilesErr  = "files-error"
	idTruncated = "files-truncated"
	idPrefix    = "commit:"
	idFile      = "file:"

	// fileDepth is the level of the rows under the Files section.
	fileDepth = 1

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

// FileID returns the ID of the row of the file at path.
func FileID(path string) string { return idFile + path }

// PathOf returns the path of the file whose row has the given ID.
func PathOf(id string) (path string, ok bool) { return strings.CutPrefix(id, idFile) }

// Data is what the rows show: the commits of the pull request, oldest first,
// the range of them that is selected, and the files that changed in it.
type Data struct {
	Commits core.PullCommits
	Range   Range
	// Files are the files changed in the range, in any order; the rows sort
	// them by path.
	Files []core.CommitFile
	// FilesErr says why the files could not be read, and shows in place of them.
	FilesErr string
	// Truncated is the note that says the list of files is cut short, or "".
	Truncated string
	// Children returns the rows that go under the commit oid when its fold is
	// open, whose top rows are at depth. With Children set, a commit that has
	// checks is a fold, closed at first; without it, no commit is.
	Children func(oid string, depth int) []foldtree.Row
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

// builder draws the rows of one pane with its options and styles.
type builder struct {
	o     Options
	st    *Styles
	width int
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
		row := b.commit(c, d.Range.Contains(i), d.Children != nil)
		rows = append(rows, row)
		if row.Fold {
			rows = append(rows, d.Children(c.OID, commitDepth+1)...)
		}
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
	return append(rows, b.files(d)...)
}

// files are the rows under the Files section: the error of the read, or a row
// for each file by path, then the note that the list is cut short.
func (b *builder) files(d Data) []foldtree.Row {
	if d.FilesErr != "" {
		return []foldtree.Row{b.note(idFilesErr, d.FilesErr)}
	}
	fs := slices.Clone(d.Files)
	slices.SortStableFunc(fs, func(x, y core.CommitFile) int { return strings.Compare(x.Path, y.Path) })
	rows := make([]foldtree.Row, 0, len(fs)+1)
	for _, f := range fs {
		rows = append(rows, b.file(f))
	}
	if d.Truncated != "" {
		rows = append(rows, b.note(idTruncated, d.Truncated))
	}
	return rows
}

// note is a dim row that the cursor skips.
func (b *builder) note(id, text string) foldtree.Row {
	return foldtree.Row{
		ID:    id,
		Depth: fileDepth,
		Lines: []string{b.st.dim.Render(termtext.OneLine(text))},
		Skip:  true,
	}
}

// file is the row of f: its icon and path, and the lines it adds and deletes
// on the right.
func (b *builder) file(f core.CommitFile) foldtree.Row {
	return foldtree.Row{
		ID:    FileID(f.Path),
		Depth: fileDepth,
		Kind:  KindFile,
		Lines: []string{b.st.fileIcon(f.Path) + termtext.OneLine(f.Path)},
		Right: b.counts(f.Additions, f.Deletions),
	}
}

// counts is "+a −d" drawn in color, leaving out a count of none.
func (b *builder) counts(add, del int) string {
	var parts []string
	if add > 0 {
		parts = append(parts, b.st.add.Render("+"+strconv.Itoa(add)))
	}
	if del > 0 {
		parts = append(parts, b.st.del.Render(b.st.Glyphs.Minus+strconv.Itoa(del)))
	}
	return strings.Join(parts, " ")
}

// Totals returns how many files fs has and the lines they add and delete.
func Totals(fs []core.CommitFile) (n, add, del int) {
	for _, f := range fs {
		add += f.Additions
		del += f.Deletions
	}
	return len(fs), add, del
}

// Info returns the forms of what the title of the pane says on its right,
// longest first: "3 files · +12 −4" and then "3 · +12 −4". It has none for no
// files.
func Info(n, add, del int, st Styles) []string {
	if n <= 0 {
		return nil
	}
	b := builder{st: &st}
	sep := st.dim.Render(" " + st.Glyphs.Dot + " ")
	rest := sep + b.totals(add, del)
	noun := " files"
	if n == 1 {
		noun = " file"
	}
	return []string{
		st.dim.Render(strconv.Itoa(n)+noun) + rest,
		st.dim.Render(strconv.Itoa(n)) + rest,
	}
}

// totals is "+A −D" in color, with both counts even when one is none.
func (b *builder) totals(add, del int) string {
	return b.st.add.Render("+"+strconv.Itoa(add)) + " " + b.st.del.Render(b.st.Glyphs.Minus+strconv.Itoa(del))
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
// range. It is a fold when checks says that the checks can open under it and c
// has some.
func (b *builder) commit(c core.PullCommit, in, checks bool) foldtree.Row {
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
		// The checks of a commit open under it, with enter.
		Fold:   checks && c.Checks != core.CheckNone,
		Closed: true,
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
