package pulls

import (
	"log/slog"
	"maps"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// viewing says whether the user viewed a file, as GitHub keeps it.
type viewing int

const (
	// notViewed is a file with no mark.
	notViewed viewing = iota
	// viewedFile is a file marked as viewed, as it is still.
	viewedFile
	// changedFile is a file that was marked as viewed and has changed since.
	changedFile
)

// viewingOf says whether file counts as viewed, by the marks read from GitHub,
// which tell by themselves that a marked file has changed since.
func (m *detailModal) viewingOf(file diff.File) viewing {
	f := m.files
	if f == nil || !f.pvIn {
		return notViewed
	}
	switch f.pv.State(file.Path) {
	case core.Viewed:
		return viewedFile
	case core.ViewedChanged:
		return changedFile
	case core.Unviewed:
	}
	return notViewed
}

// styleViewed renders the marks of the files in theme and icons.
func (f *filesState) styleViewed(theme ui.Theme, icons ui.Icons) {
	f.seen = theme.Success.Render(icons.Yes)
	f.changed = theme.Subtle.Render(icons.Changed)
}

// viewMark returns the mark that follows a file in the tree and its header in
// the diff, or empty for a file that wasn't viewed.
func (f *filesState) viewMark(file diff.File) string {
	if f.viewing == nil {
		return ""
	}
	switch f.viewing(file) {
	case viewedFile:
		return f.seen
	case changedFile:
		return f.changed
	case notViewed:
	}
	return ""
}

// viewedNote is the end of the status line of the diff: how many files are
// viewed.
func (f *filesState) viewedNote() string {
	if f.viewed == 0 {
		return ""
	}
	return strconv.Itoa(f.viewed) + " viewed"
}

// foldViewed folds the files that are viewed among files, or among all those
// read for nil, and counts them. Files not viewed are left as they are, so that
// one the user opened stays open.
func (m *detailModal) foldViewed(files []diff.File) {
	f := m.files
	if f == nil {
		return
	}
	if files == nil {
		files = f.set.list()
	}
	for _, file := range files {
		if m.viewingOf(file) == viewedFile {
			f.diff.SetCollapsed(file.Path, true)
		}
	}
	m.countViewed()
}

// countViewed counts the files read that are viewed, and draws the tree again
// with their marks.
func (m *detailModal) countViewed() {
	f := m.files
	n := 0
	for _, file := range f.set.list() {
		if m.viewingOf(file) == viewedFile {
			n++
		}
	}
	f.viewed = n
	f.tree.SetIcons(f.icon)
}

// toggleViewed marks file as viewed, which folds it, or takes the mark off,
// which opens it, and keeps the marks. With next set, a file that was marked
// moves the diff on to the next one that isn't viewed.
func (m *detailModal) toggleViewed(file diff.File, next bool) tea.Cmd {
	f := m.files
	if f == nil || m.svc == nil {
		return nil
	}
	switch {
	case !f.pvIn && f.pvFailed:
		return ui.Notify(toast.Info, "The files you viewed couldn't be read, so nothing was marked.")
	case !f.pvIn:
		return ui.Notify(toast.Info, "Still reading the files you viewed; try again in a moment.")
	case !f.pv.Supported:
		return ui.Notify(toast.Info, "This GitHub server keeps no viewed marks, so nothing was marked.")
	}
	if _, ok := f.sending[file.Path]; ok {
		// The answer to the last change of this file isn't in.
		return nil
	}
	marked := m.viewingOf(file) != viewedFile
	if f.sending == nil {
		f.sending = map[string]bool{}
	}
	f.sending[file.Path] = marked
	op := m.svc.SetViewed(m.viewedQuery(f.head), file.Path, marked)
	f.diff.SetCollapsed(file.Path, marked)
	m.showCachedViewed(f)
	m.countViewed()
	what := "unmark a file of #" + strconv.Itoa(m.number) + " viewed"
	if marked {
		what = "mark a file of #" + strconv.Itoa(m.number) + " viewed"
	}
	do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(m.subject(), op), what)
	cmds := []tea.Cmd{func() tea.Msg {
		done, _ := do().(ui.DoneMsg)
		answer := func() tea.Msg {
			return viewedSetMsg{files: f, path: file.Path, marked: marked, err: done.Err}
		}
		if done.Err == nil {
			return answer()
		}
		// The modal may be closed by now, and then nothing else would say that
		// the change was refused.
		return tea.BatchMsg{ui.Fail(what, done.Err), answer}
	}}
	if marked && next {
		if p := m.nextUnviewed(file.Path); p != "" {
			cmds = append(cmds, m.showFile(p))
		}
	}
	return tea.Batch(cmds...)
}

// nextUnviewed returns the path of the first file after the one at path that
// isn't viewed, or empty for none.
func (m *detailModal) nextUnviewed(path string) string {
	files := m.files.set.list()
	at, _ := m.files.set.index(path)
	for _, file := range files[min(at+1, len(files)):] {
		if m.viewingOf(file) != viewedFile {
			return file.Path
		}
	}
	return ""
}

// readOn reads on through the diff: it pages down, and once the end of the file
// the cursor is in shows, it marks that file viewed and moves to the next one
// not viewed, from the first file again after the last. With every file
// viewed it pages through the folded headers and, from the last file, moves to
// the first. Until the marks are read it only pages, and where they can't be
// set it moves on without marking.
func (m *detailModal) readOn() tea.Cmd {
	// A range being drawn is about lines, so reading on waits until it ends.
	if m.tray.from != nil {
		return nil
	}
	f := m.files
	file, ok := f.diff.CurrentFile()
	if !ok || !f.diff.FileEndVisible() || (!f.pvIn && !f.pvFailed) {
		// The page stops at the end of the file, so that the next press sees
		// the end of the file the cursor is in, not of the one after.
		cmd := f.diff.PageDownWithinFile()
		return tea.Batch(cmd, m.follow(), m.spinWhileLoading())
	}
	var cmds []tea.Cmd
	if m.viewingOf(file) != viewedFile && f.pvIn && f.pv.Supported {
		cmds = append(cmds, m.toggleViewed(file, false))
	}
	path := m.nextUnviewedWrapping(file.Path)
	if path == "" {
		// Every file is viewed, and folded: page on through the headers, and
		// start over at the first file once the last is reached.
		files := f.set.list()
		if len(files) == 0 {
			return tea.Batch(cmds...)
		}
		if file.Path != files[len(files)-1].Path {
			cmds = append(cmds, f.diff.PageDown(), m.follow(), m.spinWhileLoading())
			return tea.Batch(cmds...)
		}
		path = files[0].Path
	}
	cmds = append(cmds, m.showFile(path))
	return tea.Batch(cmds...)
}

// nextUnviewedWrapping returns the path of the first file after the one at
// path that isn't viewed, going on from the first file after the last, or
// empty when every file is viewed.
func (m *detailModal) nextUnviewedWrapping(path string) string {
	files := m.files.set.list()
	at, _ := m.files.set.index(path)
	for i := 1; i <= len(files); i++ {
		file := files[(at+i)%len(files)]
		if m.viewingOf(file) != viewedFile {
			return file.Path
		}
	}
	return ""
}

// viewedQuery returns the query of the marks of the files at head.
func (m *detailModal) viewedQuery(head string) pulls.ViewedQuery {
	return pulls.ViewedQuery{Repo: m.repo, Number: m.number, Head: head}
}

// viewedMsg carries the marks read for the files f shows.
type viewedMsg struct {
	files *filesState
	pv    core.PullViewed
	err   error
}

// viewedSetMsg carries GitHub's answer to a mark or unmark of the file at path.
type viewedSetMsg struct {
	files  *filesState
	path   string
	marked bool
	err    error
}

// readViewed reads the marks of the files f shows, in a command. It reads
// nothing while the head isn't known.
func (m *detailModal) readViewed(f *filesState) tea.Cmd {
	svc, ctx, q := m.svc, m.ctx, m.viewedQuery(f.head)
	if svc == nil || q.Head == "" {
		return nil
	}
	return func() tea.Msg {
		pv, err := svc.Viewed(ctx, q)
		return viewedMsg{files: f, pv: pv, err: err}
	}
}

// showCachedViewed shows the marks that are cached, if any, and counts them.
func (m *detailModal) showCachedViewed(f *filesState) {
	if m.svc == nil {
		return
	}
	if pv, ok := m.svc.CachedViewed(m.viewedQuery(f.head)); ok {
		f.pv, f.pvIn = pv, true
	}
}

// receiveViewed takes the marks that were read. A read that failed leaves the
// files with no marks, or those that were cached, and says so once.
func (m *detailModal) receiveViewed(msg viewedMsg) tea.Cmd {
	f := m.files
	if f == nil || msg.files != f {
		return nil
	}
	if msg.err != nil {
		f.pvFailed = true
		slog.WarnContext(m.ctx, "viewed files not read", "span", "tui", "err", msg.err)
		return ui.Fail("read the files you viewed of #"+strconv.Itoa(m.number), msg.err)
	}
	pv := msg.pv
	if len(f.sending) > 0 {
		// The read may have been answered before a change that is still on its
		// way, so what it says would take the change off the screen.
		states := maps.Clone(pv.States)
		if states == nil {
			states = map[string]core.ViewedState{}
		}
		for path, marked := range f.sending {
			if marked {
				states[path] = core.Viewed
			} else {
				delete(states, path)
			}
		}
		pv.States = states
	}
	f.pv, f.pvIn, f.pvFailed = pv, true, false
	m.foldViewed(nil)
	return nil
}

// viewedSet takes GitHub's answer to a mark or unmark. A refusal has been rolled
// back in the cache, so the file goes back to what the cache says, on the files
// that have restarted for the same head too; the command that sent it has shown
// why.
func (m *detailModal) viewedSet(msg viewedSetMsg) {
	delete(msg.files.sending, msg.path)
	f := m.files
	if msg.err == nil || f == nil || f.head != msg.files.head {
		return
	}
	m.showCachedViewed(f)
	f.diff.SetCollapsed(msg.path, f.pv.State(msg.path) == core.Viewed)
	m.countViewed()
}
