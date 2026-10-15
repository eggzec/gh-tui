package pulls

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// scopeKind is which changes the Files tab shows.
type scopeKind int

const (
	// scopeAll is every change of the pull request, which the tab opens on.
	scopeAll scopeKind = iota
	// scopeSince is what changed since the last visit: from the head seen
	// then to the head now.
	scopeSince
	// scopeCommit is what one commit of the pull request changed.
	scopeCommit
)

// filesScope is the changes the Files tab shows. Its zero value is all of
// them.
type filesScope struct {
	kind scopeKind
	// base is the head seen at the last visit, and commits how many commits
	// came since, for scopeSince.
	base    string
	commits int
	// sha and subject name the commit of scopeCommit.
	sha, subject string
}

// label says what the scope shows, for the title of the tab.
func (s filesScope) label() string {
	switch s.kind {
	case scopeSince:
		if s.commits > 0 {
			return "since you looked (" + plural(s.commits, "commit") + ")"
		}
		return "since you looked"
	case scopeCommit:
		return termtext.OneLine(s.sha[:min(len(s.sha), 7)] + " " + s.subject)
	case scopeAll:
	}
	return ""
}

// shows reports whether a thread or draft anchored at a belongs in the
// scope. A commit that is not the head has no threads, and what changed
// since the last visit ends at the head, so it holds those of the new side.
func (s filesScope) shows(a diff.Pos) bool {
	switch s.kind {
	case scopeCommit:
		return false
	case scopeSince:
		return a.Line == 0 || a.Side != diff.OldSide
	case scopeAll:
	}
	return true
}

// refusal says why a comment can't be made at pos, if given, in the scope,
// or is "" where it can.
func (s filesScope) refusal(pos *diff.Pos) string {
	switch {
	case s.kind == scopeCommit:
		return "Comment from All changes: a single commit is read-only."
	case s.kind == scopeSince && pos != nil && pos.Side == diff.OldSide:
		return "Comment on a new line: the changes since you looked end at the head."
	}
	return ""
}

// scopedFiles is the title of the tab: Files, and the scope if not all.
func (m *detailModal) scopedFiles() string {
	if f := m.files; f != nil && f.scope.kind != scopeAll {
		return "Files" + m.icons.Separator + f.scope.label()
	}
	return "Files"
}

// setFilesScope reads the files of the Files tab again within scope, which
// the keys that choose a scope call. The files are read at once if the tab
// hasn't read them yet.
func (m *detailModal) setFilesScope(s filesScope) tea.Cmd {
	old := m.files
	switch {
	case old == nil:
		if !m.canReadFiles() {
			return nil
		}
		return m.startFiles(nil, s)
	case old.scope == s:
		return nil
	}
	old.cancel()
	return m.startFiles(old, s)
}

// resetScope shows all the changes again, and says why in a toast.
func (m *detailModal) resetScope(why string) tea.Cmd {
	return tea.Batch(m.setFilesScope(filesScope{}), ui.Notify(toast.Info, why))
}

// reachAll shows all the changes again, with a toast, if the scope lacks what
// a jump aims at: the file at path, and the line or thread at anchor if given.
// A scope still being read counts as lacking it, since the file may not come.
func (m *detailModal) reachAll(path string, anchor *diff.Pos) tea.Cmd {
	f := m.files
	if f == nil || f.scope.kind == scopeAll {
		return nil
	}
	if _, in := f.set.index(path); in && f.diff.Done() && (anchor == nil || f.scope.shows(*anchor)) {
		return nil
	}
	return tea.Batch(m.setFilesScope(filesScope{}), ui.Notify(toast.Info, "Showing all the changes to reach it."))
}

// scopeGone resets a scope whose commits are gone, as a force push does, once
// the read of it says so.
func (m *detailModal) scopeGone() tea.Cmd {
	f := m.files
	if f == nil || f.scope.kind == scopeAll {
		return nil
	}
	err := f.diff.Err()
	gone := errors.Is(err, core.ErrRewritten) || (f.scope.kind == scopeCommit && errors.Is(err, core.ErrNotFound))
	if !gone {
		return nil
	}
	return m.resetScope("The history was rewritten, so Files shows all the changes again.")
}

// sinceScope returns the scope of what changed since the last visit, if the
// head moved on from it.
func (m *detailModal) sinceScope() (filesScope, bool) {
	c, base := &m.visit.catch, m.visit.prev.LastSeen.Head
	if c.rewritten || c.commits == 0 || base == "" {
		return filesScope{}, false
	}
	return filesScope{kind: scopeSince, base: base, commits: c.commits}, true
}

// openSince shows the Files tab on what changed since the last visit. A
// rewritten head has no such changes, so the tab shows all of them.
func (m *detailModal) openSince() tea.Cmd {
	s, ok := m.sinceScope()
	if !ok {
		return m.switchTo(filesTab)
	}
	return tea.Batch(m.setFilesScope(s), m.switchTo(filesTab))
}

// fetchScoped reads the files of scope, which come in one page.
func (m *detailModal) fetchScoped(head string, s filesScope) diff.Fetch {
	svc, repo, number := m.svc, m.repo, m.number
	return func(ctx context.Context, _ string) ([]diff.File, string, error) {
		ctx, end := obs.Begin(ctx, "pull.files.scope")
		var items []core.CommitFile
		var err error
		if s.kind == scopeSince {
			var d core.CompareDetail
			d, err = svc.RangeFiles(ctx, pulls.RangeQuery{Repo: repo, Base: s.base, Head: head})
			items = d.Files
		} else {
			var p core.Page[core.CommitFile]
			p, err = svc.CommitFiles(ctx, pulls.CommitFilesQuery{Repo: repo, SHA: s.sha})
			items = p.Items
		}
		end(err, "span", "tui", "repo", repo.String(), "number", number, "since", s.kind == scopeSince, "files", len(items))
		if err != nil {
			return nil, "", err
		}
		return diffFiles(items), "", nil
	}
}
