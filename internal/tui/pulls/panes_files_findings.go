package pulls

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// findingsRead is the read of what the checks found in the files, for the last
// commit of the range selected.
type findingsRead struct {
	// sha is the commit that was asked for, and seq counts the reads, so that
	// an answer to an earlier one is dropped. cancel ends the read in flight.
	sha    string
	seq    int
	cancel context.CancelFunc
	// byPath is what was found, once a read answered. A read that failed leaves
	// it empty, so the rows show no badges.
	byPath map[string]files.Finding
}

// findingsMsg carries the findings of the commit sha to the modal that asked
// for them.
type findingsMsg struct {
	id     int64
	seq    int
	sha    string
	byPath map[string]files.Finding
	err    error
	// again says that the read renews findings that are shown.
	again bool
}

// readFindings reads the annotations of the check runs, of GitHub Actions and
// of Apps, of the last commit of the range selected, unless it is the commit of
// the last read. A new commit ends the read in flight and drops its badges.
// What the cache keeps answers at once.
func (m *panesModal) readFindings() tea.Cmd {
	f := &m.files
	if m.checks == nil {
		return nil
	}
	_, sha, _, ok := m.rangeEnds()
	if !ok || sha == f.finds.sha {
		return nil
	}
	if f.finds.cancel != nil {
		f.finds.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	f.finds = findingsRead{sha: sha, seq: f.finds.seq + 1, cancel: cancel}
	f.checksVer++
	f.seq++
	svc, repo, id, seq := m.checks, m.repo, m.id, f.finds.seq
	if by, ok := cachedFindings(svc, repo, sha); ok {
		f.finds.byPath = by
		return nil
	}
	return func() tea.Msg {
		by, err := readFindings(ctx, svc, repo, sha, false)
		return findingsMsg{id: id, seq: seq, sha: sha, byPath: by, err: err}
	}
}

// refreshFindings reads the findings again when the poll of the checks of the
// commit they are of reports a change: a check that was running when they were
// read has moved, and its annotations may be new. Only a commit that is polled
// gets this; the others are read once. The badges stay until the new answer
// comes, and the answer to an earlier read is dropped.
func (m *panesModal) refreshFindings(msg ui.SyncMsg) tea.Cmd {
	f := &m.files
	sha := f.finds.sha
	if m.checks == nil || msg.Err != nil || sha == "" ||
		msg.Key != actionssvc.CommitChecksSyncKey(actionssvc.CommitChecksQuery{Repo: m.repo, SHA: sha}) {
		return nil
	}
	if f.finds.cancel != nil {
		f.finds.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	f.finds.cancel = cancel
	f.finds.seq++
	svc, repo, id, seq := m.checks, m.repo, m.id, f.finds.seq
	return func() tea.Msg {
		by, err := readFindings(ctx, svc, repo, sha, true)
		return findingsMsg{id: id, seq: seq, sha: sha, byPath: by, err: err, again: true}
	}
}

// findingsRecv takes the findings that a read brought. The answer to a read
// that was replaced is dropped; one that failed leaves the rows without badges,
// but a read again leaves the badges that it was to renew.
func (m *panesModal) findingsRecv(msg findingsMsg) {
	f := &m.files
	if msg.id != m.id || msg.seq != f.finds.seq || msg.sha != f.finds.sha {
		return
	}
	if msg.err != nil && (m.ctx.Err() != nil || errors.Is(msg.err, context.Canceled) || msg.again) {
		return
	}
	f.finds.byPath = msg.byPath
	f.checksVer++
	f.seq++
}

// cachedFindings returns the findings of the commit sha when the cache holds
// its checks and all their annotations.
func cachedFindings(svc ChecksService, repo core.RepoRef, sha string) (map[string]files.Finding, bool) {
	c, ok := svc.CachedCommitChecks(actionssvc.CommitChecksQuery{Repo: repo, SHA: sha, Notes: true})
	if !ok {
		return nil, false
	}
	var notes []core.Annotation
	for i := range c.Runs {
		run := c.Runs[i]
		n, ok := keptNotes(svc, notesQuery(repo, run), run)
		if !ok {
			return nil, false
		}
		notes = append(notes, n...)
	}
	return files.Findings(notes), true
}

// readFindings reads the checks of the commit sha with their annotations, and
// the pages of annotations that they leave out. With again it asks GitHub, not
// the cache.
func readFindings(ctx context.Context, svc ChecksService, repo core.RepoRef, sha string, again bool) (map[string]files.Finding, error) {
	c, err := svc.CommitChecks(ctx, actionssvc.CommitChecksQuery{Repo: repo, SHA: sha, Notes: true, Again: again})
	if err != nil {
		return nil, err
	}
	var notes []core.Annotation
	for i := range c.Runs {
		run := c.Runs[i]
		q := notesQuery(repo, run)
		n, ok := keptNotes(svc, q, run)
		if !ok {
			if n, err = readNotes(ctx, svc, q); err != nil {
				return nil, err
			}
		}
		notes = append(notes, n...)
	}
	return files.Findings(notes), nil
}
