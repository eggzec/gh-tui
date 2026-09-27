package actions

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// askRerunFailed asks to re-run the failed jobs of the run shown, or
// returns what tells the user they may not.
func (m *Modal) askRerunFailed() tea.Cmd {
	r, ok := m.doneRun()
	if !ok {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActRerun, nil); refused {
		return cmd
	}
	jobs := "the failed jobs"
	if m.jobs.loaded && m.jobs.runID == r.ID {
		switch n := jobview.FailedJobs(m.jobs.items); n {
		case 0:
			m.notice = jobview.RunName(r) + " has no failed jobs to re-run."
			return nil
		case 1:
			jobs = "1 failed job"
		default:
			jobs = strconv.Itoa(n) + " failed jobs"
		}
	}
	svc, repo, id := m.svc, m.repo, r.ID
	m.ask = &ui.Confirm{
		Question: "Re-run " + jobs + " of " + jobview.RunName(r) + "?",
		Run:      m.send("re-run the failed jobs of "+jobview.RunName(r), func() *optimistic.Op { return svc.RerunFailedJobs(repo, id) }),
	}
	return nil
}

// askRerun asks to re-run every job of the run shown, or returns what
// tells the user they may not.
func (m *Modal) askRerun() tea.Cmd {
	r, ok := m.doneRun()
	if !ok {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActRerun, nil); refused {
		return cmd
	}
	svc, repo, id := m.svc, m.repo, r.ID
	m.ask = &ui.Confirm{
		Question: "Re-run all jobs of " + jobview.RunName(r) + "?",
		Run:      m.send("re-run "+jobview.RunName(r), func() *optimistic.Op { return svc.RerunRun(repo, id) }),
	}
	return nil
}

// askRerunJob asks to re-run the job under the cursor of the jobs, or
// returns what tells the user they may not.
func (m *Modal) askRerunJob() tea.Cmd {
	r, ok := m.doneRun()
	if !ok {
		return nil
	}
	j, ok := m.jobs.selected()
	if !ok || m.jobs.runID != r.ID {
		m.notice = "Pick a job to re-run."
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActRerun, nil); refused {
		return cmd
	}
	svc, repo, runID, jobID := m.svc, m.repo, r.ID, j.ID
	m.ask = &ui.Confirm{
		Question: "Re-run " + ui.OneLine(j.Name) + " of " + jobview.RunName(r) + "?",
		Run:      m.send("re-run "+ui.OneLine(j.Name), func() *optimistic.Op { return svc.RerunJob(repo, runID, jobID) }),
	}
	return nil
}

// askCancel asks to cancel the run shown, or returns what tells the user
// they may not.
func (m *Modal) askCancel() tea.Cmd {
	if !m.hasRun {
		return nil
	}
	r := m.run
	if r.Done() || r.Status == core.RunCancelling {
		m.notice = jobview.RunName(r) + " isn't running."
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActCancelRun, nil); refused {
		return cmd
	}
	svc, repo, id := m.svc, m.repo, r.ID
	m.ask = &ui.Confirm{
		Question: "Cancel " + jobview.RunName(r) + "?",
		Run:      m.send("cancel "+jobview.RunName(r), func() *optimistic.Op { return svc.CancelRun(repo, id) }),
	}
	return nil
}

// gate decides what the viewer may do in the repository.
func (m *Modal) gate() ui.Gate {
	return ui.Gate{Repo: m.repo, Caps: m.caps}
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

// send returns what makes the change that start shows in the cache, named
// by what, once the user confirms it: the modal shows it at once, and it
// is sent.
func (m *Modal) send(what string, start func() *optimistic.Op) func() tea.Cmd {
	return func() tea.Cmd {
		op := start()
		return tea.Batch(m.fromCache(), ui.Do(m.parent, Title, ui.Refused(op), what))
	}
}

// answer takes the answer to the confirmation: yes makes the change and
// sends it, and no drops it. Other keys leave the question open.
func (m *Modal) answer(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := m.keys.Confirm.Answer(*m.ask, msg)
	if done {
		m.ask = nil
	}
	return cmd
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
