package pulls

import (
	"log/slog"
	"maps"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
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

// viewedState is what the diff knows of the marks of the files.
type viewedState struct {
	pv core.PullViewed
	// in says that the marks were read, and failed that the read didn't work.
	in, failed bool
	// sending holds the files whose change is on its way to GitHub, with the
	// mark it sets.
	sending map[string]bool
}

// viewedMsg carries the marks read for the files of the diff o.
type viewedMsg struct {
	o   *diffOverlay
	pv  core.PullViewed
	err error
}

// viewedSetMsg carries GitHub's answer to a mark or unmark of the file at path.
type viewedSetMsg struct {
	o      *diffOverlay
	path   string
	marked bool
	err    error
}

// viewedQuery returns the query of the marks of the files at the head of the
// pull request.
func (o *diffOverlay) viewedQuery() pulls.ViewedQuery { return o.m.viewedQuery() }

// viewedQuery returns the query of the marks of the files at the head of the
// pull request.
func (m *panesModal) viewedQuery() pulls.ViewedQuery {
	return pulls.ViewedQuery{Repo: m.repo, Number: m.number, Head: m.detail.HeadSHA}
}

// viewingOf says whether the file at path counts as viewed, by the marks read
// from GitHub, which tell by themselves that a marked file has changed since.
func (o *diffOverlay) viewingOf(path string) viewing { return o.vw.viewing(path) }

// viewing says whether the file at path counts as viewed, by the marks held.
func (v *viewedState) viewing(path string) viewing {
	if !v.in {
		return notViewed
	}
	switch v.pv.State(path) {
	case core.Viewed:
		return viewedFile
	case core.ViewedChanged:
		return changedFile
	case core.Unviewed:
	}
	return notViewed
}

// viewMark returns the mark that follows a file in its header in the diff, or
// empty for a file that wasn't viewed.
func (o *diffOverlay) viewMark(file diff.File) string {
	switch o.viewingOf(file.Path) {
	case viewedFile:
		return o.seen
	case changedFile:
		return o.changed
	case notViewed:
	}
	return ""
}

// foldViewed folds the file at path if it is viewed, in the diff of the whole
// patch. The diff of one file stays open, since the user asked to see it, and
// a file not viewed is left as it is, so that one the user opened stays open.
func (o *diffOverlay) foldViewed(path string) {
	if o.mode == diffview.WholePatch && o.viewingOf(path) == viewedFile {
		o.d.SetCollapsed(path, true)
	}
}

// nextUnviewed returns the path of the first file after the one at path that
// isn't viewed, or empty for none.
func (o *diffOverlay) nextUnviewed(path string) string {
	paths := o.src.paths()
	at := indexOf(paths, path)
	for _, p := range paths[min(at+1, len(paths)):] {
		if o.viewingOf(p) != viewedFile {
			return p
		}
	}
	return ""
}

// nextUnviewedWrapping returns the path of the first file after the one at
// path that isn't viewed, going on from the first file after the last, or
// empty when every file is viewed.
func (o *diffOverlay) nextUnviewedWrapping(path string) string {
	paths := o.src.paths()
	at := indexOf(paths, path)
	for i := 1; i <= len(paths); i++ {
		if p := paths[(at+i)%len(paths)]; o.viewingOf(p) != viewedFile {
			return p
		}
	}
	return ""
}

// readOn reads on through the diff: it pages down, and once the end of the file
// the cursor is in shows, it marks that file viewed and moves to the next one
// not viewed, from the first file again after the last. With every file
// viewed it pages through the folded headers of the whole patch and, from the
// last file, moves to the first. Until the marks are read it only pages, and
// where they can't be set it moves on without marking.
func (o *diffOverlay) readOn() tea.Cmd {
	file, ok := o.d.CurrentFile()
	if !ok || !o.d.FileEndVisible() || (!o.vw.in && !o.vw.failed) {
		// The page stops at the end of the file, so that the next press sees the
		// end of the file the cursor is in, not of the one after.
		return o.d.PageDownWithinFile()
	}
	var cmds []tea.Cmd
	if o.viewingOf(file.Path) != viewedFile && o.vw.in && o.vw.pv.Supported {
		cmds = append(cmds, o.toggleViewed(file.Path, false))
	}
	path := o.nextUnviewedWrapping(file.Path)
	if path == "" {
		// Every file is viewed. The whole patch has them folded: page on through
		// the headers, and start over at the first file once the last is reached.
		paths := o.src.paths()
		if len(paths) == 0 {
			return tea.Batch(cmds...)
		}
		switch {
		case o.mode == diffview.WholePatch && file.Path != paths[len(paths)-1]:
			return tea.Batch(append(cmds, o.d.PageDown())...)
		case o.mode == diffview.WholePatch:
			path = paths[0]
		default:
			path = paths[(indexOf(paths, file.Path)+1)%len(paths)]
		}
	}
	return tea.Batch(append(cmds, o.goTo(path))...)
}

// indexOf returns the place of path among paths, or -1.
func indexOf(paths []string, path string) int {
	for i, p := range paths {
		if p == path {
			return i
		}
	}
	return -1
}

// readViewed reads the marks of the files, in a command. It reads nothing while
// the head isn't known.
func (o *diffOverlay) readViewed() tea.Cmd {
	svc, ctx, q := o.m.svc, o.ctx, o.viewedQuery()
	if q.Head == "" {
		return nil
	}
	return func() tea.Msg {
		pv, err := svc.Viewed(ctx, q)
		return viewedMsg{o: o, pv: pv, err: err}
	}
}

// showCachedViewed takes the marks that are cached, if any.
func (o *diffOverlay) showCachedViewed() { o.m.cachedViewed(&o.vw) }

// cachedViewed takes the marks that are cached into vw, if any, and says
// whether there were any.
func (m *panesModal) cachedViewed(vw *viewedState) bool {
	pv, ok := m.svc.CachedViewed(m.viewedQuery())
	if ok {
		vw.pv, vw.in = pv, true
	}
	return ok
}

// receiveViewed takes the marks that were read. A read that failed leaves the
// files with no marks, or those that were cached, and says so once.
func (o *diffOverlay) receiveViewed(msg viewedMsg) tea.Cmd {
	if o.done {
		return nil
	}
	if msg.err != nil {
		o.vw.failed = true
		slog.WarnContext(o.ctx, "viewed files not read", "span", "tui", "err", msg.err)
		return ui.Fail("read the files you viewed of #"+strconv.Itoa(o.m.number), msg.err)
	}
	o.vw.receive(msg.pv)
	for _, p := range o.src.paths() {
		o.foldViewed(p)
	}
	return nil
}

// receive takes the marks that were read.
func (v *viewedState) receive(pv core.PullViewed) {
	if len(v.sending) > 0 {
		// The read may have been answered before a change that is still on its
		// way, so what it says would take the change off the screen.
		states := maps.Clone(pv.States)
		if states == nil {
			states = map[string]core.ViewedState{}
		}
		for path, marked := range v.sending {
			if marked {
				states[path] = core.Viewed
			} else {
				delete(states, path)
			}
		}
		pv.States = states
	}
	v.pv, v.in, v.failed = pv, true, false
}

// viewedSet takes GitHub's answer to a mark or unmark. A refusal has been
// rolled back in the cache, so the file goes back to what the cache says; the
// command that sent it has shown why.
func (o *diffOverlay) viewedSet(msg viewedSetMsg) {
	if o.done {
		delete(o.vw.sending, msg.path)
		return
	}
	if o.m.settleViewed(&o.vw, msg.path, msg.marked, msg.err) {
		o.setFold(msg.path, o.vw.pv.State(msg.path) == core.Viewed)
	}
}

// settleViewed takes GitHub's answer to the mark or unmark of the file at path
// in vw, and says whether it was a refusal, which it has put back.
func (m *panesModal) settleViewed(vw *viewedState, path string, marked bool, err error) bool {
	delete(vw.sending, path)
	if err == nil {
		return false
	}
	if !m.cachedViewed(vw) {
		// The cache doesn't hold the entry any more, so put back what the file had.
		vw.mark(path, !marked)
	}
	return true
}

// setFold folds the file at path, or opens it, in the diff of the whole patch.
// The diff of one file stays as it is, since the user asked to see it.
func (o *diffOverlay) setFold(path string, fold bool) {
	if o.mode == diffview.WholePatch {
		o.d.SetCollapsed(path, fold)
	}
}

// mark sets the mark of the file at path in the marks held.
func (v *viewedState) mark(path string, marked bool) {
	states := maps.Clone(v.pv.States)
	if states == nil {
		states = map[string]core.ViewedState{}
	}
	if marked {
		states[path] = core.Viewed
	} else {
		delete(states, path)
	}
	v.pv.States = states
}

// toggleViewed marks the file at path as viewed, which folds it, or takes the
// mark off, which opens it, and keeps the marks. With next set, a file that was
// marked moves the diff on to the next one that isn't viewed.
func (o *diffOverlay) toggleViewed(path string, next bool) tea.Cmd {
	cmd, marked := o.m.toggleViewed(&o.vw, path,
		func(marked bool) { o.setFold(path, marked) },
		func(marked bool, err error) tea.Msg {
			return viewedSetMsg{o: o, path: path, marked: marked, err: err}
		})
	if marked && next {
		if p := o.nextUnviewed(path); p != "" {
			return tea.Batch(cmd, o.goTo(p))
		}
	}
	return cmd
}

// toggleViewed marks the file at path as viewed in vw, or takes the mark off,
// and sends the change to GitHub. fold shows the change at once, and answer
// makes the message that carries GitHub's answer back. It says whether a mark
// was set, and does nothing but a hint where the marks can't be changed.
func (m *panesModal) toggleViewed(vw *viewedState, path string, fold func(marked bool), answer func(marked bool, err error) tea.Msg) (tea.Cmd, bool) {
	if path == "" {
		return nil, false
	}
	switch {
	case !vw.in && vw.failed:
		return ui.Notify(toast.Info, "The files you viewed couldn't be read, so nothing was marked."), false
	case !vw.in:
		return ui.Notify(toast.Info, "Still reading the files you viewed; try again in a moment."), false
	case !vw.pv.Supported:
		return ui.Notify(toast.Info, "This GitHub server keeps no viewed marks, so nothing was marked."), false
	}
	if _, ok := vw.sending[path]; ok {
		// The answer to the last change of this file isn't in.
		return nil, false
	}
	marked := vw.viewing(path) != viewedFile
	if vw.sending == nil {
		vw.sending = map[string]bool{}
	}
	vw.sending[path] = marked
	op := m.svc.SetViewed(m.viewedQuery(), path, marked)
	fold(marked)
	m.cachedViewed(vw)
	vw.mark(path, marked)
	m.syncViewed()
	what := "unmark a file of #" + strconv.Itoa(m.number) + " viewed"
	if marked {
		what = "mark a file of #" + strconv.Itoa(m.number) + " viewed"
	}
	about := core.Target{Repo: m.repo, Number: m.number}.String()
	do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(about, op), what)
	return func() tea.Msg {
		done, _ := do().(ui.DoneMsg)
		reply := func() tea.Msg { return answer(marked, done.Err) }
		if done.Err == nil {
			return reply()
		}
		// The modal may be closed by now, and then nothing else would say that
		// the change was refused.
		return tea.BatchMsg{ui.Fail(what, done.Err), reply}
	}, marked
}
