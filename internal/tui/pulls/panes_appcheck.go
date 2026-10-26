package pulls

import (
	"context"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/appcheck"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// ctxAppCheck is the context of the keys of the page of a check run of an App.
const ctxAppCheck = "pull_app_check"

// maxNotePages bounds the pages of annotations that the page reads.
const maxNotePages = 5

// appCheckKeys are the keys of the page of a check run of an App, besides the
// moves, which are those of the preview, and the choice of an annotation.
type appCheckKeys struct {
	// rerequest asks to run the check suite of the run again.
	rerequest key.Binding
}

func newAppCheckKeys(keys config.Keymap) appCheckKeys {
	return appCheckKeys{rerequest: ui.In(keys, ctxAppCheck).Binding("rerequest", "re-request")}
}

// appCheckNotesMsg carries the annotations that a read of the overlay brought.
type appCheckNotesMsg struct {
	o     *appCheckOverlay
	notes []core.Annotation
	err   error
}

// appCheckOverlay shows what a check run of an App reported over the panes.
type appCheckOverlay struct {
	m      *panesModal
	sha    string
	check  core.CommitCheck
	notes  []core.Annotation
	state  appcheck.Notes
	v      appcheck.Model
	ctx    context.Context
	cancel context.CancelFunc
	done   bool
}

// openAppCheck opens the page of the check run that msg names, from the checks
// of its commit that the modal holds. It does nothing for a run it doesn't
// know.
func (m *panesModal) openAppCheck(msg checktree.OpenCheckMsg) tea.Cmd {
	if m.checks == nil {
		return nil
	}
	cc, ok := m.checks.CachedCommitChecks(actions.CommitChecksQuery{Repo: m.repo, SHA: msg.SHA})
	if !ok {
		return nil
	}
	i := slices.IndexFunc(cc.Runs, func(c core.CommitCheck) bool { return c.ID == msg.CheckID })
	if i < 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	md := markdown.New(m.theme.Thread(m.icons).Markdown)
	md.SetGlyphs(m.icons.Markdown)
	o := &appCheckOverlay{
		m: m, sha: msg.SHA, check: cc.Runs[i], ctx: ctx, cancel: cancel,
		v: appcheck.New(appcheck.Options{Styles: appcheck.NewStyles(m.theme, m.icons), MD: md}),
	}
	cmd := o.readNotes()
	o.show()
	m.open(o)
	return cmd
}

func (o *appCheckOverlay) show() {
	o.v.SetData(appcheck.Data{Check: o.check, Notes: o.notes, State: o.state})
}

// readNotes takes the annotations that are kept, and returns the command that
// reads them when they aren't.
func (o *appCheckOverlay) readNotes() tea.Cmd {
	svc, q := o.m.checks, notesQuery(o.m.repo, o.check)
	notes, kept := keptNotes(svc, q, o.check)
	if kept {
		o.notes = notes
		return nil
	}
	o.state = appcheck.NotesLoading
	ctx := o.ctx
	return func() tea.Msg {
		notes, err := readNotes(ctx, svc, q)
		return appCheckNotesMsg{o: o, notes: notes, err: err}
	}
}

// notesQuery is the read of the annotations of the check run c of repo.
func notesQuery(repo core.RepoRef, c core.CommitCheck) actions.AnnotationsQuery {
	return actions.AnnotationsQuery{Repo: repo, CheckRunID: c.ID, PageSize: 100}
}

// keptNotes returns the annotations of c that are at hand, and reports whether
// they are all there: c has none, brought them, or the cache holds them. When
// it doesn't, they are read with q.
func keptNotes(svc ChecksService, q actions.AnnotationsQuery, c core.CommitCheck) ([]core.Annotation, bool) {
	if c.Annotations == 0 && len(c.Notes) == 0 {
		return nil, true
	}
	if len(c.Notes) > 0 && len(c.Notes) >= c.Annotations {
		return slices.Clone(c.Notes), true
	}
	return cachedNotes(svc, q)
}

func cachedNotes(svc ChecksService, q actions.AnnotationsQuery) ([]core.Annotation, bool) {
	var all []core.Annotation
	for range maxNotePages {
		p, ok := svc.CachedAnnotations(q)
		if !ok {
			return nil, false
		}
		all = append(all, p.Items...)
		if p.Next == "" {
			return all, true
		}
		q.Cursor = p.Next
	}
	return all, true
}

func readNotes(ctx context.Context, svc ChecksService, q actions.AnnotationsQuery) ([]core.Annotation, error) {
	var all []core.Annotation
	for range maxNotePages {
		p, err := svc.Annotations(ctx, q)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Items...)
		if p.Next == "" {
			break
		}
		q.Cursor = p.Next
	}
	return all, nil
}

func (o *appCheckOverlay) title() string {
	if a := o.check.App; a != nil && a.Name != "" {
		return a.Name + o.m.icons.Separator + o.check.Name
	}
	return o.check.Name
}

func (o *appCheckOverlay) kind() panes.OverlayKind { return panes.Viewer }
func (o *appCheckOverlay) items() int              { return 0 }
func (o *appCheckOverlay) text() int               { return 0 }
func (o *appCheckOverlay) clear() bool             { return false }
func (o *appCheckOverlay) closed() bool            { return o.done }

func (o *appCheckOverlay) view(w, h int) string {
	o.v.SetSize(w, h)
	return o.v.View()
}

// finish ends the read of the annotations.
func (o *appCheckOverlay) finish() {
	o.done = true
	o.cancel()
}

func (o *appCheckOverlay) layers() []keyhelp.Layer {
	k, mv := o.m.keys, o.m.keys.scroll
	l := ui.ContextLayer(ctxAppCheck,
		[]key.Binding{mv.Up, mv.Down, mv.PageUp, mv.PageDown, mv.HalfPageUp, mv.HalfPageDown, mv.Home, mv.End, k.choose, k.appCheck.rerequest},
		[]key.Binding{mv.Up, mv.Down, k.choose, k.appCheck.rerequest})
	return []keyhelp.Layer{ui.ContextLayer(ctxModal, nil, nil), l}
}

func (o *appCheckOverlay) update(msg tea.Msg) tea.Cmd {
	if n, ok := msg.(appCheckNotesMsg); ok {
		if n.o != o {
			return nil
		}
		o.notes, o.state = n.notes, appcheck.NotesLoaded
		if n.err != nil {
			o.state = appcheck.NotesFailed
			if o.ctx.Err() != nil {
				return nil
			}
		}
		o.show()
		return nil
	}
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	k, mv := o.m.keys, o.m.keys.scroll
	moves := []struct {
		b key.Binding
		s appcheck.Step
	}{
		{mv.Up, appcheck.Up}, {mv.Down, appcheck.Down},
		{mv.PageUp, appcheck.PageUp}, {mv.PageDown, appcheck.PageDown},
		{mv.HalfPageUp, appcheck.HalfPageUp}, {mv.HalfPageDown, appcheck.HalfPageDown},
		{mv.Home, appcheck.Top}, {mv.End, appcheck.Bottom},
	}
	switch {
	case keymap.Matches(press, k.appCheck.rerequest):
		return o.rerequest()
	case keymap.Matches(press, k.choose):
		return o.enter()
	}
	for _, mvt := range moves {
		if keymap.Matches(press, mvt.b) {
			o.v.Move(mvt.s)
			break
		}
	}
	return nil
}

// enter closes the page and opens the diff at the annotation under the cursor.
func (o *appCheckOverlay) enter() tea.Cmd {
	a, ok := o.v.Selected()
	if !ok || a.Path == "" {
		return nil
	}
	return o.m.openAt(a.Path, a.StartLine, o)
}

// rerequest asks whether to run the check suite of the run again.
func (o *appCheckOverlay) rerequest() tea.Cmd {
	m, c := o.m, o.check
	if c.Outcome() == core.CheckPending {
		return ui.Notify(toast.Info, c.Name+" is already running")
	}
	if c.SuiteID == 0 {
		return ui.Notify(toast.Info, c.Name+" has no check suite to re-request")
	}
	m.ask(ui.Confirm{Question: "Re-request check suite?", Run: func() tea.Cmd {
		op := ui.About(m.repo.String()+" "+c.Name, m.checks.RerequestSuite(m.repo, c.SuiteID))
		send := m.sendChange(op, mergeSentMsg{what: "re-request the checks of " + c.Name, done: "Re-requested the checks of " + c.Name})
		var reload tea.Cmd
		m.merge.loader, reload = m.merge.loader.Reload(o.sha)
		m.merge.rebuild(m, false)
		return tea.Batch(send, reload)
	}})
	return nil
}

// openAt closes the page from and opens the diff of the file at path with the
// cursor on line, of the new side. A file outside the selected commits resets
// them to every commit; one that the pull request doesn't change leaves the
// page open, with a hint.
func (m *panesModal) openAt(path string, line int, from overlay) tea.Cmd {
	f := &m.files
	has := func(fs []core.CommitFile) bool {
		return slices.ContainsFunc(fs, func(c core.CommitFile) bool { return c.Path == path })
	}
	n := len(f.commits.Commits)
	all := f.loaded && n > 0 && f.rng == files.All(n)
	// The files of the pull request are those kept, or, while every commit is
	// selected, those that the Files pane read.
	fs, known, _ := cachedPullFiles(m.svc, m.repo, m.number, m.detail.HeadSHA)
	if !known && all {
		_, _, fs, known = m.filesRange()
	}
	if known && !has(fs) {
		return ui.Notify(toast.Info, "the file "+path+" is not in this diff")
	}
	var reset tea.Cmd
	if f.loaded && n > 0 && !all {
		inRange := false
		if _, _, fs, ok := m.filesRange(); ok {
			inRange = has(fs)
		}
		if !inRange {
			f.rng = files.All(n)
			f.seq++
			reset = m.readRange()
		}
	}
	if m.top() == from {
		m.pop()
	}
	cmd := m.openDiff(path)
	if o, ok := m.top().(*diffOverlay); ok && line > 0 {
		o.d.Seek(diff.Pos{Path: path, Side: diff.NewSide, Line: line})
	}
	return tea.Batch(reset, cmd)
}
