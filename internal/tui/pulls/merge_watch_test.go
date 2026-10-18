package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// commitWatches records the follows the Merge pane starts and ends.
type commitWatches struct {
	started []actions.CommitChecksQuery
	stopped int
}

func (w *commitWatches) watch(q actions.CommitChecksQuery) func() {
	w.started = append(w.started, q)
	return func() { w.stopped++ }
}

func headChecks(sha string, status core.RunStatus, c core.Conclusion) core.CommitChecks {
	return core.CommitChecks{SHA: sha, Runs: []core.CommitCheck{{ID: 1, Name: "build", Status: status, Conclusion: c}}}
}

// watchedMerge returns a Merge pane over a head whose checks are pending and
// kept, with a recording watch.
func watchedMerge(t *testing.T) (*panesModal, *checksFake, *commitWatches) {
	t.Helper()
	m, _ := mergePanes(t, nil)
	c := &checksFake{kept: map[string]core.CommitChecks{"abc123": headChecks("abc123", core.RunInProgress, "")}}
	w := &commitWatches{}
	m.merge.watch = w.watch
	withChecks(m, c)
	return m, c, w
}

func TestMergePaneWatchesPendingChecks(t *testing.T) {
	t.Parallel()
	m, _, w := watchedMerge(t)
	if len(w.started) != 1 || w.started[0].SHA != "abc123" || w.started[0].Repo != m.repo {
		t.Fatalf("watches started = %+v, want one of the head's checks", w.started)
	}
	// Reading the pane again doesn't start another.
	m.merge.refresh(m)
	if len(w.started) != 1 {
		t.Errorf("watches started = %d after a refresh, want 1", len(w.started))
	}
}

func TestMergePaneWatchesNothingWhenDone(t *testing.T) {
	t.Parallel()
	m, c, w := watchedMerge(t)
	c.kept["abc123"] = headChecks("abc123", core.RunCompleted, core.ConclusionSuccess)
	w.started, w.stopped = nil, 0
	m.merge.unwatch()
	m.merge.refresh(m)
	if len(w.started) != 0 {
		t.Errorf("watches started = %d for checks that completed, want none", len(w.started))
	}
}

func TestMergePaneSyncShowsTheChecksAndStopsWhenDone(t *testing.T) {
	t.Parallel()
	m, c, w := watchedMerge(t)
	if readyShows(m) {
		t.Fatal("ready shows while the checks are pending")
	}
	key := actions.CommitChecksSyncKey(w.started[0])

	// Another key says nothing of these checks.
	m.Update(ui.SyncMsg{Key: "other"})
	if w.stopped != 0 {
		t.Fatal("a sync of another key ended the watch")
	}

	c.kept["abc123"] = headChecks("abc123", core.RunCompleted, core.ConclusionSuccess)
	m.Update(ui.SyncMsg{Key: key})
	if !readyShows(m) {
		t.Error("ready doesn't show once the poll found the checks passed")
	}
	if w.stopped != 1 || len(w.started) != 1 {
		t.Errorf("watches started = %d, stopped = %d; want 1 and 1", len(w.started), w.stopped)
	}
}

func TestMergePaneHeadChangeStopsTheWatch(t *testing.T) {
	t.Parallel()
	m, _, w := watchedMerge(t)
	m.detail.HeadSHA = "def789"
	m.merge.refresh(m)
	if w.stopped != 1 {
		t.Errorf("watches stopped = %d after the head changed, want 1", w.stopped)
	}
	if len(w.started) != 1 {
		t.Errorf("watches started = %d, want the new head's checks not followed before they are read", len(w.started))
	}
}

func TestMergePaneCloseStopsTheWatch(t *testing.T) {
	t.Parallel()
	m, _, w := watchedMerge(t)
	m.Discard()
	if w.stopped != 1 {
		t.Errorf("watches stopped = %d after the modal closed, want 1", w.stopped)
	}
	m.Discard()
	if w.stopped != 1 {
		t.Errorf("watches stopped = %d after a second discard, want 1", w.stopped)
	}
}
