package actions

import (
	"context"

	tea "charm.land/bubbletea/v2"

	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
)

// readJobsAround reads, once the cursor of the runs rests, the jobs of
// the runs in a window around it, as the prefetch settings say, so that
// they show at once. The modal reads the jobs of the run under the cursor
// itself, whatever they say.
func (m *Modal) readJobsAround() tea.Cmd {
	i := m.runs.Index()
	at := func(j int) (actionssvc.JobsQuery, bool) {
		r, ok := m.runs.Item(j)
		if j == i || !ok {
			return actionssvc.JobsQuery{}, false
		}
		r = m.current(r)
		return actionssvc.JobsQuery{Repo: m.repo, RunID: r.ID, Attempt: r.Attempt}, true
	}
	// Until a run is listed, there is no window.
	if _, ok := m.runs.Selected(); !ok {
		at = nil
	}
	return m.aheadJobs.Window(at, i)
}

// readJobsAhead reads the jobs of q ahead of their use, for m.aheadJobs.
func (m *Modal) readJobsAhead(ctx context.Context, q actionssvc.JobsQuery) error {
	_, err := m.svc.AllJobs(ctx, q)
	return err
}

// cachedJobs reports whether the jobs of q are in memory.
func (m *Modal) cachedJobs(q actionssvc.JobsQuery) bool {
	_, ok := m.svc.CachedAllJobs(q)
	return ok
}

// readLogsAround reads, once the cursor of the jobs rests, the logs of
// the failed jobs in a window around it, as the prefetch settings say.
// Only a job that ended has its whole log, and a failed one is the one
// likely opened; a log can run to megabytes, so the others are left. The
// log pane reads the log of the job under the cursor itself.
func (m *Modal) readLogsAround() tea.Cmd {
	j := &m.jobs
	at := func(k int) (int64, bool) {
		if k == j.cursor || k >= len(j.lines) || j.lines[k].group != nil {
			return 0, false
		}
		job := j.items[j.lines[k].job]
		return job.ID, job.Done() && job.Conclusion.Failed()
	}
	// Until the jobs are listed, there is no window.
	if !m.hasRun || !j.loaded {
		at = nil
	}
	return m.aheadLogs.Window(at, j.cursor)
}

// readLogAhead reads the log of job jobID ahead of its use, for
// m.aheadLogs.
func (m *Modal) readLogAhead(ctx context.Context, jobID int64) error {
	_, err := m.svc.Log(ctx, m.repo, jobID)
	return err
}

// cachedLog reports whether the log of job jobID is in memory.
func (m *Modal) cachedLog(jobID int64) bool {
	_, ok := m.svc.CachedLog(m.repo, jobID)
	return ok
}
