package actions

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// maxNoteJobs bounds how many jobs of a run have their annotations read, so
// that a run of a big matrix doesn't spend the rate limit on one pane.
const maxNoteJobs = 20

// noteRes is what was read of the annotations of one job: the first page of
// them.
type noteRes struct {
	items   []core.Annotation
	more    bool
	loading bool
	err     error
	// kept reports that the page was served from what an earlier read kept,
	// because GitHub couldn't be reached, so it is read again once it answers.
	kept bool
}

// noteEntry is a row of the annotations pane: an annotation and the job it
// belongs to.
type noteEntry struct {
	job core.Job
	a   core.Annotation
}

// notesMsg carries the annotations of a job.
type notesMsg struct {
	id    int64
	jobID int64
	page  core.Page[core.Annotation]
	err   error
}

func (m *Modal) noteQuery(j core.Job) actionssvc.AnnotationsQuery {
	// The ID of a job is that of its check run.
	return actionssvc.AnnotationsQuery{Repo: m.repo, CheckRunID: j.ID}
}

// noteJobs are the jobs of the run shown whose annotations are listed: those
// that ended, the failed ones first.
func (m *Modal) noteJobs() []core.Job {
	j := m.cur()
	if !m.hasRun || !j.loaded || j.runID != m.run.ID {
		return nil
	}
	var out []core.Job
	for i := range j.items {
		if j.items[i].Done() {
			out = append(out, j.items[i])
		}
	}
	slices.SortStableFunc(out, func(a, b core.Job) int {
		switch fa, fb := a.Conclusion.Failed(), b.Conclusion.Failed(); {
		case fa && !fb:
			return -1
		case fb && !fa:
			return 1
		}
		return 0
	})
	return out[:min(len(out), maxNoteJobs)]
}

// readNotes shows from memory the annotations of the jobs of the run shown
// that aren't read yet, and reads the others.
func (m *Modal) readNotes() tea.Cmd {
	var cmds []tea.Cmd
	js := m.noteJobs()
	for i := range js {
		j := &js[i]
		if m.ann[j.ID] != nil {
			continue
		}
		if m.ann == nil {
			m.ann = map[int64]*noteRes{}
		}
		if p, ok := m.svc.CachedAnnotations(m.noteQuery(*j)); ok {
			m.ann[j.ID] = &noteRes{items: p.Items, more: p.Next != ""}
			continue
		}
		m.ann[j.ID] = &noteRes{loading: true}
		cmds = append(cmds, m.readNote(*j))
	}
	return tea.Batch(cmds...)
}

func (m *Modal) readNote(j core.Job) tea.Cmd {
	svc, ctx, id, q := m.svc, m.ctx, m.id, m.noteQuery(j)
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.annotations")
		p, err := svc.Annotations(ctx, q)
		end(err, "span", "tui", "check_run", q.CheckRunID, "annotations", len(p.Items))
		return notesMsg{id: id, jobID: j.ID, page: p, err: err}
	}
}

func (m *Modal) receiveNotes(msg notesMsg) {
	r := m.ann[msg.jobID]
	if r == nil {
		return
	}
	r.loading = false
	if msg.err != nil {
		// Rows from an earlier answer stay, with the error shown beside them.
		r.err = msg.err
		return
	}
	r.err, r.items, r.more = nil, msg.page.Items, msg.page.Next != ""
	r.kept = msg.page.Offline || msg.page.Limited
}

// rereadNotes reads again the annotations that failed to load, or were kept
// for want of an answer from GitHub.
func (m *Modal) rereadNotes() tea.Cmd {
	var cmds []tea.Cmd
	js := m.noteJobs()
	for i := range js {
		j := &js[i]
		if r := m.ann[j.ID]; r != nil && !r.loading && (r.err != nil || r.kept) {
			// The cache holds a kept page as if it were fresh, so the read
			// goes to the service.
			// The rows stay until the answer.
			r.loading, r.err, r.kept = true, nil, false
			cmds = append(cmds, m.readNote(*j))
		}
	}
	return tea.Batch(append(cmds, m.readNotes())...)
}

// noteEntries lists the annotations of the jobs shown, those of the failed
// jobs first.
func (m *Modal) noteEntries() []noteEntry {
	var out []noteEntry
	js := m.noteJobs()
	for i := range js {
		if r := m.ann[js[i].ID]; r != nil {
			for _, a := range r.items {
				out = append(out, noteEntry{job: js[i], a: a})
			}
		}
	}
	return out
}

// noteState reports whether any annotations are loading, whether a job has
// more than the page read, and the first error of those that failed.
func (m *Modal) noteState() (loading, more bool, err error) {
	js := m.noteJobs()
	for i := range js {
		r := m.ann[js[i].ID]
		if r == nil {
			continue
		}
		more = more || r.more
		switch {
		case r.loading:
			loading = true
		case r.err != nil && err == nil:
			err = r.err
		}
	}
	return loading, more, err
}

// noteNotice is what the rows lack, as lines under the title: the error of a
// read that failed, while the annotations of the other jobs list.
func (m *Modal) noteNotice(w int) []string {
	if len(m.noteEntries()) == 0 {
		return nil
	}
	if _, _, err := m.noteState(); err != nil {
		return m.errorLines("load the annotations", jobview.RunName(m.run), err, w)
	}
	return nil
}

// notesInner is the size inside the annotations box.
func (m *Modal) notesInner() (w, h int) {
	aw, ah := m.area()
	if b, ok := paneLayout(aw, ah, paneNotes, m.zoom).Box(paneNotes, panes.Whole); ok {
		in := b.Rect.Inner()
		return in.W, in.H
	}
	return 0, 0
}

// noteSplit trims the facts of the run to leave the annotations room in h
// lines, and returns them with how many annotation rows fit under their
// title, once extra lines of notice are kept.
func noteSplit(facts []string, n, extra, h int) (kept []string, rows int) {
	want := 2 + min(n, 3) + extra
	if n == 0 {
		want = 3
	}
	if len(facts)+want > h {
		facts = facts[:max(min(len(facts), h-want), 0)]
	}
	lead := 0
	if len(facts) > 0 {
		lead = 1
	}
	return facts, min(n, max(h-len(facts)-lead-1-extra, 0))
}

// scrollTop keeps the cursor within rows rows from top.
func scrollTop(top, cursor, rows, n int) int {
	rows = max(rows, 1)
	top = min(top, cursor)
	if cursor >= top+rows {
		top = cursor - rows + 1
	}
	return max(min(top, n-rows), 0)
}

// pressNotes moves the cursor of the annotations.
func (m *Modal) pressNotes(msg tea.KeyPressMsg) tea.Cmd {
	n := len(m.noteEntries())
	if n == 0 {
		return nil
	}
	c := m.noteCursor
	switch {
	case keymap.Matches(msg, m.keys.notesUp):
		c--
	case keymap.Matches(msg, m.keys.notesDown):
		c++
	default:
		return nil
	}
	m.noteCursor = min(max(c, 0), n-1)
	w, h := m.notesInner()
	_, rows := noteSplit(m.runFacts(w), n, len(m.noteNotice(w)), h)
	m.noteTop = scrollTop(m.noteTop, m.noteCursor, rows, n)
	return nil
}

// openNote opens the log of the job of the annotation under the cursor.
func (m *Modal) openNote() tea.Cmd {
	es := m.noteEntries()
	if m.noteCursor < 0 || m.noteCursor >= len(es) {
		return nil
	}
	e := es[m.noteCursor]
	return m.openLog(e.job, jobview.Hints{Find: noteFind(e.a)})
}

// noteFind lists what the log of a job may have printed of annotation a, for
// the log to open on: the file and line a compiler or linter prints, else the
// first line of the message. It is a guess, which the log may not bear out.
func noteFind(a core.Annotation) []string {
	var find []string
	if a.Path != "" && a.StartLine > 0 {
		find = append(find, a.Path+":"+strconv.Itoa(a.StartLine))
	}
	if msg, _, _ := strings.Cut(strings.TrimSpace(a.Message), "\n"); msg != "" {
		find = append(find, msg)
	}
	return find
}

// noteLines renders what follows the facts of the run in the annotations
// box: the title and the rows, or why there are none, in h lines of w cells.
func (m *Modal) noteLines(w, h, rows int) []string {
	st := &m.st
	es := m.noteEntries()
	loading, _, err := m.noteState()
	cur := m.cur()
	loading = loading || !cur.loaded && cur.err == nil
	title := st.Subtle.Render(st.ic.Comment) + " " + st.Strong.Render("Annotations")
	if loading && len(es) > 0 {
		title += " " + st.Subtle.Render("loading"+m.st.ic.Ellipsis)
	}
	lines := []string{ui.Fit(title, w)}
	switch {
	case len(es) > 0:
		lines = append(lines, m.noteNotice(w)...)
		c := min(m.noteCursor, len(es)-1)
		top := scrollTop(m.noteTop, c, rows, len(es))
		for i := top; i < len(es) && i < top+rows; i++ {
			lines = append(lines, m.noteRow(es[i], i == c, w))
		}
	case err != nil:
		lines = append(lines, m.errorLines("load the annotations", jobview.RunName(m.run), err, w)...)
	case loading:
		lines = append(lines, st.Muted.Render("Loading annotations"+m.st.ic.Ellipsis))
	case cur.err != nil && !cur.loaded:
	default:
		lines = append(lines, st.Muted.Render("No annotations on this run. Pick another run, or press the refresh key to read again."))
	}
	return lines[:min(len(lines), max(h, 0))]
}

// notesCount is the forms of the count at the bottom of the annotations box.
// A job with more than a page lists its first page only, hence the "+".
func (m *Modal) notesCount() []string {
	if !m.hasRun {
		return nil
	}
	_, more, _ := m.noteState()
	return countForms(len(m.noteEntries()), more, "annotation", "annotations")
}

// noteRow renders an annotation: its level, where it points, its message,
// and the job it belongs to, dim, at the right.
func (m *Modal) noteRow(e noteEntry, cursor bool, w int) string {
	st := &m.st
	a := e.a
	var glyph string
	switch a.Level {
	case core.AnnotationFailure:
		glyph = st.Glyphs[ui.RunFailure]
	case core.AnnotationWarning:
		glyph = st.Warning.Render(st.ic.Warning)
	default:
		glyph = st.Muted.Render(st.ic.Info)
	}
	where := ""
	if a.Path != "" && a.Path != ".github" && a.StartLine > 0 {
		where = st.Accent.Render(ui.OneLine(a.Path)+":"+strconv.Itoa(a.StartLine)) + " "
	}
	text := ui.OneLine(ui.FirstLine(a.Message))
	if a.Title != "" && a.Title != text {
		text = ui.OneLine(a.Title) + ": " + text
	}
	left := m.rowGutter(cursor, m.notes) + glyph + " " + where + st.Text.Render(text)
	job := st.Subtle.Render(ui.OneLine(e.job.Name))
	return m.spreadWhole(left, job, w)
}
