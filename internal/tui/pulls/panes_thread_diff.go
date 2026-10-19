package pulls

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// openThread opens the diff on the file of thread t, with the cursor on the
// line it is about and the thread under it. A thread that is outdated opens the
// diff of the pull request at the commit it was made on, to be read against the
// code it was about.
func (m *panesModal) openThread(t *core.Thread) tea.Cmd {
	if !outdatedThread(t) {
		o, cmd := m.openDiffOf(t.Path, m.diffSource(), "")
		o.seekThread(t)
		return cmd
	}
	commit := threadCommit(t)
	if commit == "" {
		o, cmd := m.openDiffOf(t.Path, m.diffSource(), "")
		o.seekThread(t)
		return tea.Batch(cmd, ui.Notify(toast.Info, "The commit of this thread is no longer available."))
	}
	svc, repo, base := m.svc, m.repo, m.detail.BaseSHA
	src := newDiffSource(func(ctx context.Context) ([]core.CommitFile, error) {
		d, err := svc.RangeFiles(ctx, pulls.RangeQuery{Repo: repo, Base: base, Head: commit})
		return d.Files, err
	})
	o, cmd := m.openDiffOf(t.Path, src, commit)
	o.seekThread(t)
	return cmd
}

// seekThread puts the cursor on the line of thread t that the diff shows it on:
// the top of its range. A thread of the file, or one whose line the diff doesn't
// show, leaves the cursor on the header of the file.
func (o *diffOverlay) seekThread(t *core.Thread) {
	line, start := o.threadSpan(t)
	if t.File || line <= 0 {
		return
	}
	side := t.Side
	if start > 0 && start < line {
		line = start
		if t.StartSide != "" {
			side = t.StartSide
		}
	}
	o.d.Seek(diff.Pos{Path: t.Path, Side: threadSide(side), Line: line})
}
