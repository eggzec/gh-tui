package history

import (
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// labelWidth is the width of the names of the header's fields.
const labelWidth = 10

// View renders the panes side by side, or the focused one alone with a
// breadcrumb on a narrow terminal, in exactly the size of the last SetSize.
func (m *Modal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	h := m.bodyHeight()
	if m.narrow() {
		lines := make([]string, 0, m.height)
		lines = append(lines, fit(m.breadcrumb(m.width), m.width))
		return strings.Join(append(lines, m.paneLines(m.focus, m.width, h)...), "\n")
	}
	bw, gw, cw := m.widths()
	cols := [numPanes][]string{
		append([]string{m.paneTitle(branchPane, bw)}, m.paneLines(branchPane, bw, h)...),
		append([]string{m.paneTitle(graphPane, gw)}, m.paneLines(graphPane, gw, h)...),
		append([]string{m.paneTitle(commitPane, cw)}, m.paneLines(commitPane, cw, h)...),
	}
	var b strings.Builder
	b.Grow(m.height * (m.width + 128))
	for i := range m.height {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(cols[0][i])
		b.WriteString(m.st.sep)
		b.WriteString(cols[1][i])
		b.WriteString(m.st.sep)
		b.WriteString(cols[2][i])
	}
	return b.String()
}

// bodyHeight is the height of a pane below its title, or its breadcrumb.
func (m *Modal) bodyHeight() int {
	return max(m.height-1, 0)
}

// widths returns the widths of the branch, graph and commit panes. On a
// narrow terminal each takes the whole width, one at a time.
func (m *Modal) widths() (branches, graph, commit int) {
	if m.narrow() {
		return m.width, m.width, m.width
	}
	branches = min(max(m.width/5, 16), 26)
	rest := m.width - branches - 2*sepWidth
	commit = rest * 45 / 100
	return branches, rest - commit, commit
}

func (m *Modal) paneWidth(p pane) int {
	b, g, c := m.widths()
	return [numPanes]int{b, g, c}[p]
}

// layout sizes the bubbles to their panes.
func (m *Modal) layout() {
	h := m.bodyHeight()
	m.graph.model.SetSize(m.paneWidth(graphPane), h)
	if f := m.branches.filter; f != nil {
		f.SetSize(m.paneWidth(branchPane), h)
	}
	m.scrollBranches()
	m.layoutCommit()
}

// paneLines renders the body of pane p, h lines of w cells.
func (m *Modal) paneLines(p pane, w, h int) []string {
	switch p {
	case branchPane:
		return m.branchLines(w, h)
	case graphPane:
		// The graph renders its size exactly.
		return padLines(strings.Split(m.graph.model.View(), "\n"), w, h)
	case commitPane:
	}
	return m.commitLines(w, h)
}

// paneTitle renders the first line of pane p: what it shows, in the accent
// while it has the focus.
func (m *Modal) paneTitle(p pane, w int) string {
	st := m.st.title
	if p == m.focus {
		st = m.st.focusTitle
	}
	var text, detail string
	switch p {
	case branchPane:
		text = "Branches"
		if n := len(m.branches.items); n > 0 {
			detail = strconv.Itoa(n)
			if m.branches.next != "" {
				detail += "+"
			}
		}
	case graphPane:
		text = m.graphName()
	case commitPane:
		text = "Commit"
		if c := &m.commit; c.has {
			text, detail = short(c.c.SHA), oneLine(c.c.Subject)
		}
	}
	text = ansi.Truncate(oneLine(text), w, "…")
	line := st.Render(text)
	if room := w - ansi.StringWidth(text) - 1; detail != "" && room > 1 {
		line += " " + m.st.subtle.Render(ansi.Truncate(detail, room, "…"))
	}
	return fit(line, w)
}

// breadcrumb shows where the narrow modal is: the branches, the branch,
// the commit and the file, as far as the focus went.
func (m *Modal) breadcrumb(w int) string {
	crumbs := []string{"Branches"}
	if m.focus >= graphPane {
		crumbs = append(crumbs, m.graphName())
	}
	if c := &m.commit; m.focus == commitPane && c.has {
		crumbs = append(crumbs, short(c.c.SHA))
		if c.patch && c.cursor < len(c.files) {
			crumbs = append(crumbs, path.Base(c.files[c.cursor].Path))
		}
	}
	for i := range crumbs {
		crumbs[i] = oneLine(crumbs[i])
	}
	const sep = " › "
	// Drop the first crumbs until the rest fit.
	for len(crumbs) > 1 && ansi.StringWidth(strings.Join(crumbs, sep)) > w {
		crumbs = crumbs[1:]
		if crumbs[0] != "…" {
			crumbs = append([]string{"…"}, crumbs[1:]...)
		}
	}
	var b strings.Builder
	for i, c := range crumbs {
		if i > 0 {
			b.WriteString(m.st.subtle.Render(sep))
		}
		st := m.st.crumb
		if i == len(crumbs)-1 {
			st = m.st.lastCrumb
		}
		b.WriteString(st.Render(c))
	}
	return ansi.Truncate(b.String(), w, "…")
}

// commitGeom is how the commit pane shares its height: the header, the
// files, and the pager below them, which shows the file under the cursor.
type commitGeom struct {
	header, files, pager int
	// cut is how many lines of the header are left out.
	cut int
}

func (m *Modal) commitGeom() commitGeom {
	c := &m.commit
	h := m.bodyHeight()
	if c.patch {
		return commitGeom{pager: h}
	}
	header := len(m.headerLines(m.paneWidth(commitPane)))
	// With a patch below them, the header and the files take half the
	// pane at most; without, the header leaves the files a third.
	most := max(3, h/2)
	if m.narrow() {
		most = max(3, h-min(len(c.files)+1, max(3, h/3)))
	}
	g := commitGeom{header: min(header, most, h)}
	if header > g.header && g.header > 0 {
		g.cut = header - g.header + 1
	}
	rest := h - g.header
	rows := len(c.files) + 1
	if m.narrow() || !c.loaded || len(c.files) == 0 || rest < 12 {
		g.files = rest
		return g
	}
	g.files = min(rows, max(3, rest/3))
	g.pager = rest - g.files - 1
	return g
}

// layoutCommit sizes the pager to its place in the commit pane.
func (m *Modal) layoutCommit() {
	if g := m.commitGeom(); g.pager > 0 {
		m.commit.pager.SetSize(m.paneWidth(commitPane), g.pager)
	}
	m.scrollFiles()
}

// pagerShown reports whether the commit pane shows the pager.
func (m *Modal) pagerShown() bool {
	return m.commitGeom().pager > 0
}

// filesHeight is the height of the list of files.
func (m *Modal) filesHeight() int {
	return m.commitGeom().files
}

// commitLines renders the commit pane's body, h lines of w cells.
func (m *Modal) commitLines(w, h int) []string {
	c := &m.commit
	if c.patch {
		return padLines(strings.Split(c.pager.View(), "\n"), w, h)
	}
	if !c.has {
		return fitLines([]string{m.st.muted.Render("Pick a commit to see what it changed.")}, w, h)
	}
	g := m.commitGeom()
	lines := make([]string, 0, h)
	header := m.headerLines(w)
	if g.cut > 0 {
		lines = append(lines, header[:g.header-1]...)
		more := "… " + strconv.Itoa(g.cut) + " more lines"
		if k := m.keys.Open.Help().Key; k != "" {
			more += " · " + k + " opens it on GitHub"
		}
		lines = append(lines, fit(m.st.subtle.Render(ansi.Truncate(more, w, "…")), w))
	} else {
		lines = append(lines, header...)
	}
	lines = append(lines, padLines(m.fileLines(w, g.files), w, g.files)...)
	if g.pager > 0 {
		lines = append(lines, m.st.subtle.Render(strings.Repeat("─", w)))
		lines = append(lines, strings.Split(c.pager.View(), "\n")...)
	}
	// Every part is rendered to the width already.
	return padLines(lines, w, h)
}

// fileLines renders the list of files, or what stands for it until the
// detail is loaded.
func (m *Modal) fileLines(w, h int) []string {
	c := &m.commit
	switch {
	case c.err != nil:
		return wrap(m.errorLine("Couldn't load the changes: ", c.err), w, "")
	case !c.loaded:
		return []string{fit(m.spin.View()+m.st.muted.Render("Loading the changes…"), w)}
	case len(c.files) == 0:
		return []string{fit(m.st.muted.Render("This commit changed no files."), w)}
	}
	lines := make([]string, 0, h)
	focused := m.focus == commitPane
	for i := c.top; i < len(c.files) && len(lines) < h; i++ {
		lines = append(lines, m.fileRow(c.files[i], i == c.cursor, focused, w))
	}
	if len(lines) < h {
		switch {
		case c.filesErr != nil:
			lines = append(lines, wrap(m.errorLine("Couldn't load more files: ", c.filesErr), w, m.st.noGutter)...)
		case c.filesLoading:
			lines = append(lines, fit(m.st.noGutter+m.spin.View()+m.st.muted.Render("Loading more files…"), w))
		case c.next == "" && c.truncated:
			text := "GitHub lists " + strconv.Itoa(core.MaxCommitFiles) + " files at most."
			if k := m.keys.Open.Help().Key; k != "" {
				text += " " + k + " shows them all."
			}
			lines = append(lines, wrap(m.st.subtle.Render(text), w, m.st.noGutter)...)
		}
	}
	return lines
}

// errorLine renders an error that the retry key reads again.
func (m *Modal) errorLine(what string, err error) string {
	text := m.st.error.Render("✗ " + what + trim(err.Error()))
	if k := m.keys.Retry.Help().Key; k != "" {
		text += m.st.subtle.Render(" · " + k + " to retry")
	}
	return text
}

// wrap wraps s to lines of w cells, each after indent.
func wrap(s string, w int, indent string) []string {
	iw := ansi.StringWidth(indent)
	lines := strings.Split(ansi.Wrap(s, max(w-iw, 1), ""), "\n")
	for i, l := range lines {
		lines[i] = fit(indent+l, w)
	}
	return lines
}

// fileRow renders one changed file: how it changed, its path, and the
// lines it added and removed.
func (m *Modal) fileRow(f core.CommitFile, cursor, focused bool, w int) string {
	gutter := m.st.noGutter
	if cursor {
		gutter = m.st.blurGutter
		if focused {
			gutter = m.st.gutter
		}
	}
	letter, st := "M", m.st.warning
	switch f.Status {
	case core.FileAdded:
		letter, st = "A", m.st.success
	case core.FileRemoved:
		letter, st = "D", m.st.error
	case core.FileRenamed:
		letter, st = "R", m.st.accent
	case core.FileCopied:
		letter, st = "C", m.st.accent
	case core.FileUnchanged:
		letter, st = "·", m.st.subtle
	case core.FileModified, core.FileChanged:
	}
	adds, dels := "+"+strconv.Itoa(f.Additions), "−"+strconv.Itoa(f.Deletions)
	countsW := ansi.StringWidth(adds) + 1 + ansi.StringWidth(dels)
	name := oneLine(f.Path)
	if f.PreviousPath != "" && f.PreviousPath != f.Path {
		name = oneLine(f.PreviousPath) + " → " + name
	}
	room := w - 4
	showCounts := room-countsW > 8
	if showCounts {
		room -= countsW + 1
	}
	name = truncateLeft(name, room)
	line := gutter + st.Render(letter) + " " + m.st.text.Render(name)
	if !showCounts {
		return fit(line, w)
	}
	pad := w - 4 - ansi.StringWidth(name) - countsW
	return line + strings.Repeat(" ", max(pad, 1)) + m.st.success.Render(adds) + " " + m.st.error.Render(dels)
}

// truncateLeft cuts s to w cells from the left, so that a path keeps its
// file name.
func truncateLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw <= w {
		return s
	}
	return "…" + ansi.TruncateLeft(s, sw-w+1, "")
}

// headerLines returns the rendered header of the commit shown, at width w,
// cached until the commit, the width or the detail change.
func (m *Modal) headerLines(w int) []string {
	c := &m.commit
	if !c.has {
		return nil
	}
	k := headerKey{sha: c.c.SHA, width: w, loaded: c.loaded}
	if c.header == nil || c.headerKey != k {
		c.header, c.headerKey = m.renderHeader(w), k
	}
	return c.header
}

// renderHeader renders the configured fields of the commit shown.
func (m *Modal) renderHeader(w int) []string {
	c := &m.commit
	k := c.c
	f := m.format
	now := m.now()
	var lines []string
	// A value too long for the pane wraps below itself, past the labels.
	indent := strings.Repeat(" ", labelWidth)
	field := func(label, value string) {
		for i, l := range wrap(value, w, indent) {
			if i == 0 {
				l = m.st.label.Render(pad(label, labelWidth)) + ansi.TruncateLeft(l, labelWidth, "")
			}
			lines = append(lines, l)
		}
	}
	both := slices.Contains(f.detail, config.FieldAuthor) && slices.Contains(f.detail, config.FieldCommitter)
	shared := both && samePerson(k.Author, k.Committer)
	for _, name := range f.detail {
		switch name {
		case config.FieldSHA:
			field("Commit", m.st.muted.Render(k.SHA))
		case config.FieldAuthor:
			field("Author", m.st.text.Render(f.person(k.Author)))
		case config.FieldCommitter:
			if !shared {
				field("Committer", m.st.text.Render(f.person(k.Committer)))
			}
		case config.FieldDate:
			field("Date", m.st.text.Render(f.dates(k.Author.Date, k.Committer.Date, now)))
		case config.FieldVerification:
			text, ok, signed := verification(k.Verification)
			st := m.st.muted
			switch {
			case ok:
				st = m.st.success
			case signed:
				st = m.st.error
			}
			field("Signature", st.Render(text))
		case config.FieldParents:
			ps := make([]string, len(k.Parents))
			for i, p := range k.Parents {
				ps[i] = short(p)
			}
			text := strings.Join(ps, " ")
			if text == "" {
				text = "none, the first commit"
			}
			field("Parents", m.st.muted.Render(text))
		case config.FieldTrailers:
			for i, t := range k.Trailers {
				label := ""
				if i == 0 {
					label = "Trailers"
				}
				field(label, m.st.muted.Render(oneLine(t.Key)+":")+" "+m.st.text.Render(f.trailerValue(t.Value)))
			}
		case config.FieldStats:
			if !c.loaded {
				continue
			}
			s := c.detail.Stats
			n := strconv.Itoa(len(c.detail.Files))
			if c.detail.FilesNext != "" {
				n += "+"
			}
			noun := " files"
			if n == "1" {
				noun = " file"
			}
			field("Changes", m.st.success.Render("+"+strconv.Itoa(s.Additions))+" "+
				m.st.error.Render("−"+strconv.Itoa(s.Deletions))+m.st.muted.Render(" in "+n+noun))
		case config.FieldBody:
			lines = m.appendBody(lines, k.Body, w)
		}
	}
	return lines
}

// appendBody appends the body of a message, wrapped to w, after a blank
// line.
func (m *Modal) appendBody(lines []string, body string, w int) []string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\t", "    "))
	if body == "" {
		return lines
	}
	lines = append(lines, strings.Repeat(" ", w))
	for l := range strings.SplitSeq(reflow(body), "\n") {
		wrapped := ansi.Wrap(oneLine(l), w, "")
		for part := range strings.SplitSeq(wrapped, "\n") {
			lines = append(lines, fit(m.st.text.Render(ansi.Truncate(part, w, "")), w))
		}
	}
	return lines
}

// reflow joins the lines of each paragraph of a message, which git wraps
// at about 72 columns, so that it wraps to the pane instead. Lines that
// start a list item or are indented, such as code, keep their breaks.
func reflow(body string) string {
	lines := strings.Split(body, "\n")
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			if joins(lines[i-1], l) {
				b.WriteByte(' ')
			} else {
				b.WriteByte('\n')
			}
		}
		b.WriteString(strings.TrimRight(l, " "))
	}
	return b.String()
}

// joins reports whether line continues the paragraph of prev.
func joins(prev, line string) bool {
	if strings.TrimSpace(prev) == "" || strings.TrimSpace(line) == "" {
		return false
	}
	switch r := line[0]; {
	case r == ' ', r == '-', r == '*', r == '>', r >= '0' && r <= '9':
		return false
	}
	return prev[0] != ' '
}

// pad pads s with spaces to w cells.
func pad(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// fit pads or cuts s to w cells.
func fit(s string, w int) string {
	sw := ansi.StringWidth(s)
	switch {
	case sw == w:
		return s
	case sw < w:
		return s + strings.Repeat(" ", w-sw)
	}
	return ansi.Truncate(s, w, "")
}

// padLines returns exactly h of lines, which are w cells wide already:
// the lines past h are left out, and blank ones fill the rest.
func padLines(lines []string, w, h int) []string {
	h = max(h, 0)
	if len(lines) >= h {
		return lines[:h]
	}
	blank := strings.Repeat(" ", w)
	for len(lines) < h {
		lines = append(lines, blank)
	}
	return lines
}

// fitLines returns exactly h lines of exactly w cells: lines cut or padded.
func fitLines(lines []string, w, h int) []string {
	out := make([]string, max(h, 0))
	for i := range out {
		if i < len(lines) {
			out[i] = fit(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}
