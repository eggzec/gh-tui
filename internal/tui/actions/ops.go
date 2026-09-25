package actions

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// confirm is a change of the run shown that waits for the user to say
// yes, on the last line of the modal.
type confirm struct {
	// question asks, such as "Re-run 2 failed jobs of CI #4812?", and what
	// names the change for the error toast.
	question, what string
	// start makes the change in the cache and returns the Op that sends it.
	start func() *optimistic.Op
}

// askRerunFailed asks to re-run the failed jobs of the run shown.
func (m *Modal) askRerunFailed() {
	r, ok := m.doneRun()
	if !ok {
		return
	}
	jobs := "the failed jobs"
	if m.jobs.loaded && m.jobs.runID == r.ID {
		switch n := jobview.FailedJobs(m.jobs.items); n {
		case 0:
			m.notice = jobview.RunName(r) + " has no failed jobs to re-run."
			return
		case 1:
			jobs = "1 failed job"
		default:
			jobs = strconv.Itoa(n) + " failed jobs"
		}
	}
	svc, repo, id := m.svc, m.repo, r.ID
	m.ask = &confirm{
		question: "Re-run " + jobs + " of " + jobview.RunName(r) + "?",
		what:     "re-run the failed jobs of " + jobview.RunName(r),
		start:    func() *optimistic.Op { return svc.RerunFailedJobs(repo, id) },
	}
}

// askRerun asks to re-run every job of the run shown.
func (m *Modal) askRerun() {
	r, ok := m.doneRun()
	if !ok {
		return
	}
	svc, repo, id := m.svc, m.repo, r.ID
	m.ask = &confirm{
		question: "Re-run all jobs of " + jobview.RunName(r) + "?",
		what:     "re-run " + jobview.RunName(r),
		start:    func() *optimistic.Op { return svc.RerunRun(repo, id) },
	}
}

// askRerunJob asks to re-run the job under the cursor of the jobs.
func (m *Modal) askRerunJob() {
	r, ok := m.doneRun()
	if !ok {
		return
	}
	j, ok := m.jobs.selected()
	if !ok || m.jobs.runID != r.ID {
		m.notice = "Pick a job to re-run."
		return
	}
	svc, repo, runID, jobID := m.svc, m.repo, r.ID, j.ID
	m.ask = &confirm{
		question: "Re-run " + ui.OneLine(j.Name) + " of " + jobview.RunName(r) + "?",
		what:     "re-run " + ui.OneLine(j.Name),
		start:    func() *optimistic.Op { return svc.RerunJob(repo, runID, jobID) },
	}
}

// askCancel asks to cancel the run shown.
func (m *Modal) askCancel() {
	if !m.hasRun {
		return
	}
	r := m.run
	if r.Done() || r.Status == core.RunCancelling {
		m.notice = jobview.RunName(r) + " isn't running."
		return
	}
	svc, repo, id := m.svc, m.repo, r.ID
	m.ask = &confirm{
		question: "Cancel " + jobview.RunName(r) + "?",
		what:     "cancel " + jobview.RunName(r),
		start:    func() *optimistic.Op { return svc.CancelRun(repo, id) },
	}
}

// doneRun returns the run shown, if it completed, as a re-run needs.
func (m *Modal) doneRun() (core.Run, bool) {
	if !m.hasRun {
		return core.Run{}, false
	}
	if !m.run.Done() {
		m.notice = jobview.RunName(m.run) + " is still running."
		if k := m.keys.Cancel.Help().Key; k != "" {
			m.notice += " " + k + " cancels it."
		}
		return core.Run{}, false
	}
	return m.run, true
}

// answer takes the answer to the confirmation: yes makes the change and
// sends it, and anything else drops it.
func (m *Modal) answer(msg tea.KeyPressMsg) tea.Cmd {
	ask := m.ask
	m.ask = nil
	if !key.Matches(msg, m.keys.Yes) {
		return nil
	}
	op := ask.start()
	return tea.Batch(m.fromCache(), ui.Do(m.parent, Title, ui.Refused(op), ask.what))
}

// done re-renders from the cache once a change was confirmed or rolled
// back. A confirmed change leaves the run stale, so it is read again,
// unless the sync engine follows it and reads it anyway.
func (m *Modal) done(msg ui.DoneMsg) tea.Cmd {
	cmd := m.fromCache()
	if msg.Err != nil || (m.opts.follow != nil && m.following == m.run.ID) {
		return cmd
	}
	return tea.Batch(cmd, m.readRun())
}
