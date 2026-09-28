package jobview

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// notes are the annotations of the job shown: the first page of them,
// which holds more than the view has room for.
type notes struct {
	items []core.Annotation
	// more reports that the job has more than the page.
	more bool
	// loading is set while they are read, loaded once they arrived, and
	// err once they failed to.
	loading, loaded bool
	err             error
	cursor, top     int
}

// scroll keeps the cursor within rows rows from the top.
func (n *notes) scroll(rows int) {
	rows = max(rows, 1)
	n.top = min(n.top, n.cursor)
	if n.cursor >= n.top+rows {
		n.top = n.cursor - rows + 1
	}
	n.top = max(min(n.top, len(n.items)-rows), 0)
}

// wantsNotes reports whether the job shown has annotations worth reading:
// only a failed job's point at why it failed.
func (m *Model) wantsNotes() bool {
	return m.job.Done() && m.job.Conclusion.Failed() && !m.hints.NoAnnotations
}

func (m *Model) notesQuery() actionssvc.AnnotationsQuery {
	return actionssvc.AnnotationsQuery{Repo: m.repo, CheckRunID: m.job.ID}
}

// cachedNotes shows the annotations from memory, and reports whether
// there is nothing left to read.
func (m *Model) cachedNotes() bool {
	if !m.wantsNotes() {
		return true
	}
	p, ok := m.svc.CachedAnnotations(m.notesQuery())
	if ok {
		m.setNotes(p)
	}
	return ok
}

// notesMsg carries the annotations of a job.
type notesMsg struct {
	id    int64
	jobID int64
	page  core.Page[core.Annotation]
	err   error
}

// readNotes reads the annotations of the job shown, if it has any worth
// reading.
func (m *Model) readNotes() tea.Cmd {
	if !m.wantsNotes() {
		return nil
	}
	m.notes.loading = true
	svc, ctx, id, q := m.svc, m.ctx, m.id, m.notesQuery()
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.annotations")
		p, err := svc.Annotations(ctx, q)
		end(err, "span", "tui", "check_run", q.CheckRunID, "annotations", len(p.Items))
		return notesMsg{id: id, jobID: q.CheckRunID, page: p, err: err}
	}
}

func (m *Model) receiveNotes(msg notesMsg) {
	if msg.jobID != m.job.ID || m.state == None {
		return
	}
	defer m.layout()
	m.notes.loading = false
	if msg.err != nil {
		m.notes.err = msg.err
		return
	}
	m.setNotes(msg.page)
}

func (m *Model) setNotes(p core.Page[core.Annotation]) {
	m.notes.items, m.notes.more = p.Items, p.Next != ""
	m.notes.loaded, m.notes.loading, m.notes.err = true, false, nil
	m.notes.cursor = min(m.notes.cursor, max(len(p.Items)-1, 0))
}

// notesHeight is how many lines the annotations take above the log: a
// title, and a few of them.
func (m *Model) notesHeight() int {
	switch {
	case m.notes.err != nil:
		return 1
	case len(m.notes.items) == 0:
		return 0
	}
	return 1 + m.noteRows()
}

// noteRows is how many annotations show at once: a few, so that the log
// keeps most of the room.
func (m *Model) noteRows() int {
	return min(len(m.notes.items), min(max(m.height/5, 2), 6))
}

// press takes the keys that move the focus to the annotations and back,
// and those of the annotations while they have it, passing on to the log
// the ones that don't need its cursor.
func (m *Model) press(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k, n := m.keys, &m.notes
	if len(n.items) == 0 || m.view.Capturing() {
		return nil, false
	}
	if key.Matches(msg, k.Annotations) {
		m.focusLog(m.onNotes)
		return nil, true
	}
	if !m.onNotes {
		return nil, false
	}
	// The keys that change how the whole log shows need none of its
	// cursor, so they work from here too.
	if key.Matches(msg, m.wholeLog()...) {
		m.view.Focus()
		var cmd tea.Cmd
		m.view, cmd = m.view.Update(msg)
		m.view.Blur()
		return cmd, true
	}
	switch {
	case key.Matches(msg, k.Up):
		n.cursor = max(n.cursor-1, 0)
	case key.Matches(msg, k.Down):
		n.cursor = min(n.cursor+1, len(n.items)-1)
	case key.Matches(msg, k.Select):
		return m.openNote(), true
	}
	n.scroll(m.noteRows())
	// The log keeps still while the annotations have the keys.
	return nil, true
}

// wholeLog returns the keys that change how the whole log shows.
func (m *Model) wholeLog() []key.Binding {
	lk := m.view.KeyMap()
	return []key.Binding{lk.FoldAll, lk.Wrap, lk.Times, lk.LineNumbers, lk.Follow}
}

// openNote previews the file of the annotation under the cursor on its
// line, at the commit the job ran on, in place of the modal the view is
// in, which the preview reopens when it closes.
func (m *Model) openNote() tea.Cmd {
	a, ok := m.selectedNote()
	if !ok || !inFile(a) || m.hints.SHA == "" {
		return nil
	}
	msg := ui.OpenFileMsg{Repo: m.repo, Path: a.Path, Ref: m.hints.SHA, Line: a.StartLine, Return: m.opts.ret}
	return func() tea.Msg { return msg }
}

func (m *Model) selectedNote() (core.Annotation, bool) {
	n := &m.notes
	if n.cursor < 0 || n.cursor >= len(n.items) {
		return core.Annotation{}, false
	}
	return n.items[n.cursor], true
}

// inFile reports whether a points at a file of the repository. GitHub
// Actions puts what is about the workflow, such as a step's exit code, on
// .github, with no line.
func inFile(a core.Annotation) bool {
	return a.Path != "" && a.Path != ".github" && a.StartLine > 0
}

// noteLines renders the annotations, notesHeight lines of w cells.
func (m *Model) noteLines(w int) []string {
	st, n := &m.st, &m.notes
	if n.err != nil {
		text := st.Error.Render("✗ Couldn't load the annotations: " + ui.FirstLine(n.err.Error()))
		return []string{ui.Fit(ansi.Truncate(text, w, "…"), w)}
	}
	if len(n.items) == 0 {
		return nil
	}
	count := strconv.Itoa(len(n.items))
	if n.more {
		count += "+"
	}
	title := st.Strong.Render("Annotations") + " " + st.Subtle.Render(count)
	var hint string
	switch k := m.keys; {
	case m.OnAnnotations() && k.Select.Help().Key != "":
		hint = k.Select.Help().Key + " opens the file · " + k.Annotations.Help().Key + " log"
	case k.Annotations.Help().Key != "":
		hint = k.Annotations.Help().Key + " to pick one"
	}
	lines := []string{ui.Spread(title, st.Subtle.Render(hint), w)}
	rows := m.noteRows()
	for i := n.top; i < len(n.items) && i < n.top+rows; i++ {
		lines = append(lines, m.noteRow(n.items[i], i == n.cursor, w))
	}
	return lines
}

// noteRow renders an annotation: its level, where it points, and its
// message.
func (m *Model) noteRow(a core.Annotation, cursor bool, w int) string {
	st := &m.st
	gutter := "  "
	if cursor && m.OnAnnotations() {
		gutter = st.Accent.Render("▌") + " "
	}
	var glyph string
	switch a.Level {
	case core.AnnotationFailure:
		glyph = st.Glyphs[ui.RunFailure]
	case core.AnnotationWarning:
		glyph = st.Warning.Render("!")
	default:
		glyph = st.Muted.Render("i")
	}
	where := ""
	if inFile(a) {
		where = st.Accent.Render(ui.OneLine(a.Path)+":"+strconv.Itoa(a.StartLine)) + " "
	}
	text := ui.OneLine(ui.FirstLine(a.Message))
	if a.Title != "" && a.Title != text {
		text = ui.OneLine(a.Title) + ": " + text
	}
	line := gutter + glyph + " " + where + st.Text.Render(text)
	return ui.Fit(ansi.Truncate(line, w, "…"), w)
}
