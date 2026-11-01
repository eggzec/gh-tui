package checktree

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// Query names the checks of one commit to read.
type Query struct {
	Repo core.RepoRef
	// SHA is the full SHA of the commit, which the read needs.
	SHA string
	// Notes asks for the annotations of the check runs that have some.
	Notes bool
}

// Reader reads the checks of one commit. The tui adapts the service that
// knows them to it.
type Reader interface {
	// CachedCommitChecks returns what is kept of the checks of q, and does no
	// I/O.
	CachedCommitChecks(q Query) (core.CommitChecks, bool)
	// CommitChecks reads the checks of q.
	CommitChecks(ctx context.Context, q Query) (core.CommitChecks, error)
}

// nextID gives each loader an identity that its messages carry.
var nextID atomic.Int64

// loadedMsg is the answer to the read of the checks of a commit.
type loadedMsg struct {
	id     int64
	sha    string
	checks core.CommitChecks
	err    error
}

// entry is what a loader knows of one commit.
type entry struct {
	loading bool
	checks  core.CommitChecks
	err     error
	notes   bool
	// cancel ends the read that is under way, if one is.
	cancel context.CancelFunc
}

// Loader keeps the checks of the commits whose rows opened, and reads them
// when a row first asks. It is a value like a model: [Loader.Want] and
// [Loader.Update] return the next one.
type Loader struct {
	id      int64
	reader  Reader
	repo    core.RepoRef
	entries map[string]*entry
}

// NewLoader returns a loader that reads the checks of the commits of repo
// with r.
func NewLoader(r Reader, repo core.RepoRef) Loader {
	return Loader{id: nextID.Add(1), reader: r, repo: repo}
}

// Want asks for the checks of the commit sha, with their annotations if notes
// is set. A commit that is loaded, loading, or failed for a reason that
// reading again would not mend, asks for nothing. What the reader keeps comes
// at once, with no command; the rest is read by the returned command.
func (l Loader) Want(sha string, notes bool) (Loader, tea.Cmd) {
	if _, ok := l.entries[sha]; ok {
		return l, nil
	}
	q := Query{Repo: l.repo, SHA: sha, Notes: notes}
	if c, ok := l.reader.CachedCommitChecks(q); ok {
		return l.with(sha, &entry{checks: c, notes: notes}), nil
	}
	return l.start(sha, notes)
}

// Reload takes up what the reader keeps of the commit sha, which it holds
// already, such as after a poll stored newer checks. A commit that was never
// asked for, or that is loading, is left to [Loader.Want] and its read. If the
// reader keeps nothing of it, the returned command reads it again.
func (l Loader) Reload(sha string) (Loader, tea.Cmd) {
	e, ok := l.entries[sha]
	if !ok || e.loading {
		return l, nil
	}
	if c, ok := l.reader.CachedCommitChecks(Query{Repo: l.repo, SHA: sha, Notes: e.notes}); ok {
		return l.with(sha, &entry{checks: c, notes: e.notes}), nil
	}
	return l.start(sha, e.notes)
}

// start marks sha as loading and returns the command that reads it.
func (l Loader) start(sha string, notes bool) (Loader, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	q := Query{Repo: l.repo, SHA: sha, Notes: notes}
	r, id := l.reader, l.id
	cmd := func() tea.Msg {
		c, err := r.CommitChecks(ctx, q)
		return loadedMsg{id: id, sha: sha, checks: c, err: err}
	}
	return l.with(sha, &entry{loading: true, notes: notes, cancel: cancel}), cmd
}

// with returns a loader that holds e for sha. It copies the entries, which
// copies of the loader share.
func (l Loader) with(sha string, e *entry) Loader {
	n := make(map[string]*entry, len(l.entries)+1)
	maps.Copy(n, l.entries)
	n[sha] = e
	l.entries = n
	return l
}

// Update handles the answer to a read of this loader, reads again the
// commits whose last read got no answer from GitHub once it answers again,
// and takes up what is kept of the loaded ones when the modal that holds the
// loader opens again.
func (l Loader) Update(msg tea.Msg) (Loader, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.id != l.id {
			return l, nil
		}
		e, ok := l.entries[msg.sha]
		if !ok || !e.loading {
			return l, nil
		}
		e.cancel()
		return l.with(msg.sha, &entry{checks: msg.checks, err: msg.err, notes: e.notes}), nil
	case ui.OnlineMsg:
		var cmds []tea.Cmd
		for sha, e := range maps.Clone(l.entries) {
			if e.loading || !ui.Unreached(e.err) {
				continue
			}
			var cmd tea.Cmd
			l, cmd = l.start(sha, e.notes)
			cmds = append(cmds, cmd)
		}
		return l, tea.Batch(cmds...)
	case ui.ReopenedMsg:
		// A modal opened from here may have changed the checks, such as by
		// re-running a job, so what is kept replaces what was read. A loaded
		// commit is never asked for again by Want.
		for sha, e := range maps.Clone(l.entries) {
			if e.loading || e.err != nil {
				continue
			}
			if c, ok := l.reader.CachedCommitChecks(Query{Repo: l.repo, SHA: sha, Notes: e.notes}); ok {
				l = l.with(sha, &entry{checks: c, notes: e.notes})
			}
		}
		return l, nil
	}
	return l, nil
}

// Handles reports whether msg is the answer to a read of this loader, which
// changes what [Loader.Rows] returns, so that a caller can draw again only then.
func (l Loader) Handles(msg tea.Msg) bool {
	m, ok := msg.(loadedMsg)
	return ok && m.id == l.id
}

// Stop ends the reads that are under way, for a loader that is no longer
// used.
func (l Loader) Stop() {
	for _, e := range l.entries {
		if e.loading {
			e.cancel()
		}
	}
}

// Rows returns the rows that stand for the checks of the commit sha: a note
// while they load, the error if they failed, and their tree once read. A
// commit that was never asked for has none.
func (l Loader) Rows(sha string, o Options, st Styles) ([]treeview.Node, Targets) {
	e, ok := l.entries[sha]
	switch {
	case !ok:
		return nil, nil
	case e.loading:
		return []treeview.Node{l.note(o, "loading", st.dim.Render("Loading checks"+st.Glyphs.Ellipsis))}, nil
	case e.err != nil:
		return l.failed(e.err, o, st), nil
	}
	return Rows(e.checks, o, st)
}

// failed returns the lines of the error err as rows.
func (l Loader) failed(err error, o Options, st Styles) []treeview.Node {
	var hint string
	switch {
	case errors.Is(err, core.ErrRateLimited):
		hint = "Rate limited: reading again once the limit lifts"
	case ui.Unreached(err):
		hint = "Offline: reading again once GitHub answers"
	}
	lines := ui.ErrorLine(st.Errors, "Couldn't read the checks", hint, o.Width)
	rows := make([]treeview.Node, len(lines))
	for i, line := range lines {
		rows[i] = l.note(o, "error/"+strconv.Itoa(i), line)
	}
	return rows
}

// note is a row that the cursor skips, with text.
func (l Loader) note(o Options, id, text string) treeview.Node {
	return treeview.Node{ID: o.Prefix + id, Depth: o.Depth, Lines: []string{text}, Skip: true}
}
