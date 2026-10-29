package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// shownThenMoved opens the modal on its first run, then moves to the next,
// and returns the ID of the run it left.
func shownThenMoved(t *testing.T) (m *Modal, h *host, left int64) {
	t.Helper()
	m, h = newModal(t, newFake(), wideW, wideH)
	left = m.run.ID
	// Headers of groups may lie between two runs.
	for range 4 {
		if m.run.ID != left {
			break
		}
		h.keys("j")
	}
	if m.run.ID == left {
		t.Fatal("j didn't move to another run")
	}
	return m, h, left
}

func TestJobsAreKeptPerRun(t *testing.T) {
	t.Parallel()
	m, _, left := shownThenMoved(t)
	if j := m.jobsOf[left]; j == nil || len(j.items) == 0 {
		t.Errorf("the jobs of the run left were lost: %+v", j)
	}
	if j := m.jobsOf[m.run.ID]; j == nil || j.runID != m.run.ID {
		t.Errorf("the run shown has no entry of its own: %+v", j)
	}
	if m.cur() != m.jobsOf[m.run.ID] {
		t.Error("cur isn't the entry of the run shown")
	}
}

func TestJobsOfAnotherRunLandInItsEntry(t *testing.T) {
	t.Parallel()
	m, h, left := shownThenMoved(t)
	shown := len(m.cur().items)
	h.send(jobsMsg{id: m.id, runID: left, attempt: m.jobsOf[left].attempt, page: core.Page[core.Job]{Items: []core.Job{{ID: 1, Name: "only"}}}})
	if got := len(m.jobsOf[left].items); got != 1 {
		t.Errorf("the entry of the run left has %d jobs, want 1", got)
	}
	if len(m.cur().items) != shown {
		t.Error("jobs of another run changed the run shown")
	}
}

func TestStaleAttemptIsDroppedPerRun(t *testing.T) {
	t.Parallel()
	m, h, left := shownThenMoved(t)
	before := len(m.jobsOf[left].items)
	h.send(jobsMsg{id: m.id, runID: left, attempt: m.jobsOf[left].attempt + 1, page: core.Page[core.Job]{Items: []core.Job{{ID: 1}}}})
	if got := len(m.jobsOf[left].items); got != before {
		t.Errorf("a stale attempt changed the entry: %d jobs, want %d", got, before)
	}
}

func TestReloadDropsRunsThatLeftTheFeed(t *testing.T) {
	t.Parallel()
	m, h, left := shownThenMoved(t)
	m.jobsOf[999999] = &jobs{runID: 999999}
	h.keys("k")
	m.dropJobsOf()
	if _, ok := m.jobsOf[999999]; ok {
		t.Error("the entry of a run not in the feed stayed")
	}
	if m.jobsOf[left] == nil || m.jobsOf[m.run.ID] == nil {
		t.Error("an entry of a listed run was dropped")
	}
}
