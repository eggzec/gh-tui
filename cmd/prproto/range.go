package main

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The Files pane is one tree: a Commits group whose rows select a contiguous
// range, and a Files group of what changed in that range.

type fcommit struct{ sha, subj, who, ago string }

var fcommits = []fcommit{
	{"a1b2c3d", "Add calc helper", "Alice Chen", "3d"},
	{"b2c3d4e", "Add tests for Div", "Alice Chen", "3d"},
	{"c3d4e5f", "Add package doc", "Alice Chen", "2d"},
	{"d4e5f6a", "Wire calc into main", "Bob Lee", "1d"},
	{"e5f6a7b", "Address review comments", "Alice Chen", "5h"},
}

// lastReview is the first commit pushed after the viewer's last review.
const lastReview = 2

var viewedGlyph = glyph{"", "✓", dim}

// hunkCommit says which commit made a hunk (the fake data owns hunks by a rule).
func hunkCommit(fi, h int) int { return (fi + h) % len(fcommits) }

func (m *model) inRange(c int) bool { return c >= m.lo && c <= m.hi }

func (m *model) allSelected() bool { return m.lo == 0 && m.hi == len(fcommits)-1 }

// rangeHunks is the diff of file fi limited to the selected commits.
func (m *model) rangeHunks(fi int) (text string, add, del int, n int) {
	var parts []string
	for h, hk := range parseHunks(files[fi].diff) {
		if !m.inRange(hunkCommit(fi, h)) {
			continue
		}
		n++
		parts = append(parts, hk.hdr+"\n"+strings.Join(hk.lines, "\n"))
		for _, l := range hk.lines {
			switch {
			case strings.HasPrefix(l, "+"):
				add++
			case strings.HasPrefix(l, "-"):
				del++
			}
		}
	}
	return strings.Join(parts, "\n"), add, del, n
}

func (m *model) fileIn(fi int) bool {
	_, _, _, n := m.rangeHunks(fi)
	return n > 0
}

func (m *model) rangeFile(fi int) file {
	f := files[fi]
	f.diff, f.add, f.del, _ = m.rangeHunks(fi)
	return f
}

func (m *model) rangeTotals() (nf, add, del int) {
	for i := range files {
		if _, a, d, n := m.rangeHunks(i); n > 0 {
			nf++
			add += a
			del += d
		}
	}
	return
}

func (m *model) viewedCount() int {
	c := 0
	for i, f := range files {
		if m.viewed[f.path()] && m.fileIn(i) {
			c++
		}
	}
	return c
}

func (m *model) rangePos(fi int) (pos, tot int) {
	for i := range files {
		if m.fileIn(i) {
			tot++
			if i <= fi {
				pos = tot
			}
		}
	}
	return max(pos, 1), tot
}

func (m *model) rangeShas() string {
	if m.lo == m.hi {
		return fcommits[m.lo].sha
	}
	return fcommits[m.lo].sha + "…" + fcommits[m.hi].sha
}

func (m *model) rangeLabel() string {
	if m.allSelected() {
		return fmt.Sprintf("all %d commits · %s", len(fcommits), m.rangeShas())
	}
	return fmt.Sprintf("%d of %d commits · %s", m.hi-m.lo+1, len(fcommits), m.rangeShas())
}

// rebuild lays the tree out again for the current range and folds, keeping the
// cursor on the same row when it still exists.
func (m *model) rebuild() {
	var old row
	had := m.cur[pFiles] < len(m.rows)
	if had {
		old = m.rows[m.cur[pFiles]]
	}
	rows := []row{{kind: 'g'}}
	if m.foldOpen("g:commits", false) {
		for i, c := range fcommits {
			if i == lastReview {
				rows = append(rows, row{kind: 'm', depth: 1})
			}
			rows = append(rows, row{kind: 'c', ci: i, depth: 1})
			if m.foldOpen("c:"+c.sha, false) {
				wf := ""
				for _, r := range commitChecks(c.sha) {
					w, _, _ := strings.Cut(r.label, " / ")
					if w != wf {
						wf = w
						rows = append(rows, row{kind: 'w', ci: i, label: w, depth: 2})
					}
					if m.foldOpen("w:"+c.sha+"|"+w, true) {
						rows = append(rows, row{kind: 'k', ci: i, label: r.label, depth: 3})
					}
				}
			}
		}
	}
	rows = append(rows, row{kind: 'r'}, row{kind: 'G'})
	if m.foldOpen("g:files", true) {
		rows = append(rows, m.buildFileRows(1)...)
	}
	m.rows = rows
	cur := min(m.cur[pFiles], len(rows)-1)
	if had {
		if k := slices.Index(rows, old); k >= 0 {
			cur = k
		}
	}
	m.cur[pFiles] = m.snap(cur, 0)
}

// fileRowLine draws a row after its tree guides: first for its own line, body
// for a row that is not a node.
func (m *model) fileRowLine(r row, cw int, first, body string) string {
	ind := first
	switch r.kind {
	case 'r':
		return dim(strings.Repeat("─", max(0, cw)))
	case 'g':
		// A section title, not a tree node: no glyph, the fold shows in what follows.
		open := m.foldOpen("g:commits", false)
		t := ind + bold("Commits") + " "
		if !open && m.allSelected() {
			return t + dim(fmt.Sprintf("all %d commits ⋯", len(fcommits)))
		}
		t += dim(fmt.Sprintf("%d of %d · %s", m.hi-m.lo+1, len(fcommits), m.rangeShas()))
		if !open {
			t += dim(" ⋯")
		}
		return t
	case 'G':
		nf, a, d := m.rangeTotals()
		t := ind + bold("Files") + dim(fmt.Sprintf(" %d", nf)) + dim(" · ") + green(fmt.Sprintf("+%d", a)) + " " + red(fmt.Sprintf("−%d", d))
		if v := m.viewedCount(); v > 0 {
			t += dim(fmt.Sprintf(" · %d viewed", v))
		}
		if !m.foldOpen("g:files", true) {
			t += dim(" ⋯")
		}
		return t
	case 'm':
		return body + dim("── your last review ──")
	case 'c':
		c := fcommits[r.ci]
		glyph, sha, subj := dim(commitGlyph), boldDim(c.sha), dim(c.subj)
		if m.inRange(r.ci) {
			glyph, sha, subj = accent(commitGlyph), boldAccent(c.sha), c.subj
		}
		left := ind + glyph + " " + sha + " " + checkIcon(commitState(c.sha)) + " " + subj
		return rightAlign(left, dim(c.who+"  "+c.ago), cw)
	case 'k':
		ck := runOf(fcommits[r.ci].sha, r.label)
		_, job, _ := strings.Cut(r.label, " / ")
		return rightAlign(ind+checkIcon(ck.state)+" "+job, dim(stateWord(ck.state)+" · "+ck.dur), cw)
	case 'w':
		sha := fcommits[r.ci].sha
		arrow := "▸ "
		if m.foldOpen("w:"+sha+"|"+r.label, true) {
			arrow = "▾ "
		}
		return ind + dim(arrow) + r.label
	}
	if r.dir {
		arrow := "▾ "
		if !m.foldOpen("d:"+r.path, true) {
			arrow = "▸ "
		}
		return ind + dim(arrow) + r.label + dim("/")
	}
	f := m.rangeFile(r.fi)
	counts := green(fmt.Sprintf("+%d", f.add)) + " " + red(fmt.Sprintf("−%d", f.del))
	if m.viewed[f.path()] {
		return ind + dim(f.name+" "+ansi.Strip(counts)) + " " + viewedGlyph.String()
	}
	t := ind + f.name + " " + counts
	if f.thread {
		t += " " + ic.unresolved.String()
	}
	if changedSinceViewed[f.name] {
		t += " " + ic.rerequested.String() + dim(" changed since viewed")
	}
	return t
}

var changedSinceViewed = map[string]bool{"calc.go": true}

// filesSpace is the "toggle the thing under the cursor" key.
func (m *model) filesSpace() {
	r := m.rows[m.cur[pFiles]]
	switch {
	case r.kind == 'c':
		m.toggleCommit(r.ci)
	case r.isFile():
		p := files[r.fi].path()
		m.viewed[p] = !m.viewed[p]
	default:
		m.hint = "space selects a commit or marks a file viewed"
	}
	m.rebuild()
	// Move on to the next item of the same kind, so space can go down a list.
	for j := m.cur[pFiles] + 1; j < len(m.rows); j++ {
		if n := m.rows[j]; r.kind == 'c' && n.kind == 'c' || r.isFile() && n.isFile() {
			m.cur[pFiles] = j
			break
		}
	}
}

const commitGlyph = "" // nf-oct-git_commit

var boldAccent = func(t string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true).Render(t)
}

// rowFold says whether a Files row is a fold, with the key and default of its
// state.
func (m *model) rowFold(r row) (key string, def, ok bool) {
	if r.dir {
		return "d:" + r.path, true, true
	}
	switch r.kind {
	case 'g':
		return "g:commits", false, true
	case 'G':
		return "g:files", true, true
	case 'c':
		sha := fcommits[r.ci].sha
		return "c:" + sha, false, len(commitChecks(sha)) > 0
	case 'w':
		return "w:" + fcommits[r.ci].sha + "|" + r.label, true, true
	}
	return "", false, false
}

// selectable tells whether the cursor can rest on a row of the Files tree.
func (r row) selectable() bool { return r.kind != 'm' && r.kind != 'r' }

// filesRight is l in the tree: open a closed node, step into an open one.
// It reports false on a leaf, which leaves the key to the pane.
func (m *model) filesRight() bool {
	cur := m.cur[pFiles]
	r := m.rows[cur]
	key, def, ok := m.rowFold(r)
	if !ok {
		return false
	}
	if !m.foldOpen(key, def) {
		m.toggleFold(key, def)
		m.rebuild()
		return true
	}
	for j := cur + 1; j < len(m.rows) && m.rows[j].depth > r.depth; j++ {
		if m.rows[j].selectable() {
			m.cur[pFiles] = j
			break
		}
	}
	return true
}

// filesLeft is h in the tree: close an open node, else go to the parent.
func (m *model) filesLeft() {
	cur := m.cur[pFiles]
	r := m.rows[cur]
	if key, def, ok := m.rowFold(r); ok && m.foldOpen(key, def) {
		m.toggleFold(key, def)
		m.rebuild()
		return
	}
	d := r.depth
	for j := cur - 1; j >= 0 && d > 0; j-- {
		if m.rows[j].depth < d {
			d = m.rows[j].depth
			if m.rows[j].selectable() {
				m.cur[pFiles] = j
				return
			}
		}
	}
}

// toggleCommit keeps the selection one contiguous range, since GitHub can
// only diff such a range.
func (m *model) toggleCommit(i int) {
	switch {
	case i < m.lo:
		m.lo = i
	case i > m.hi:
		m.hi = i
	case m.lo == m.hi:
		m.hint = "at least one commit stays selected · A selects all"
	case i == m.lo:
		m.lo++
	case i == m.hi:
		m.hi--
	default:
		m.lo, m.hi = i, i
		m.hint = "a range has to be contiguous: only this commit is selected now"
	}
}

// focusCommit shows only one commit in the Files pane.
func (m *model) focusCommit(sha string) {
	i := slices.IndexFunc(fcommits, func(c fcommit) bool { return c.sha == sha })
	if i < 0 {
		m.hint = "unknown commit " + sha
		return
	}
	m.lo, m.hi = i, i
	if m.fold == nil {
		m.fold = map[string]bool{}
	}
	m.fold["g:commits"] = true
	m.focus = pFiles
	m.rebuild()
	for k, r := range m.rows {
		if r.kind == 'c' && r.ci == i {
			m.cur[pFiles] = k
		}
	}
}
