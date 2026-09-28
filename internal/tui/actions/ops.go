package actions

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// change is a change to a run, asked as a question: target tells which
// run or job it changes, and name names it in a note.
type change struct {
	ask          ui.Confirm
	target, name string
}

// asks asks the change that now returns on the last line of the modal, or
// shows why there is none: the notice, or the refusal it returns. By the
// time the user says yes, the run may have changed, such as re-run or
// finished elsewhere, so it asks now again, and makes the change only if
// it is still the one asked of the same run or job.
func (m *Modal) asks(now func() (c change, notice string, refusal tea.Cmd)) tea.Cmd {
	c, notice, refusal := now()
	if c.ask.Question == "" {
		if notice != "" {
			m.notice = notice
		}
		return refusal
	}
	ask := ui.Recheck(c.ask.Question, c.name, func() (ui.Confirm, bool, tea.Cmd) {
		again, _, refusal := now()
		return again.ask, again.ask.Question != "" && again.target == c.target, refusal
	})
	m.ask = &ask
	return nil
}

// rerunFailed re-runs the failed jobs of the run shown, or says why not.
func (m *Modal) rerunFailed() (change, string, tea.Cmd) {
	r, notice := m.doneRun()
	if notice != "" || !m.hasRun {
		return change{}, notice, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActRerun, nil); refused {
		return change{}, "", cmd
	}
	name := jobview.RunName(r)
	jobs := "the failed jobs"
	if m.jobs.loaded && m.jobs.runID == r.ID {
		switch n := jobview.FailedJobs(m.jobs.items); n {
		case 0:
			return change{}, name + " has no failed jobs to re-run.", nil
		case 1:
			jobs = "1 failed job"
		default:
			jobs = strconv.Itoa(n) + " failed jobs"
		}
	}
	svc, repo, id := m.svc, m.repo, r.ID
	return change{
		ask: ui.Confirm{
			Question: "Re-run " + jobs + " of " + name + "?",
			Run:      m.send("re-run the failed jobs of "+name, func() *optimistic.Op { return svc.RerunFailedJobs(repo, id) }),
		},
		target: runTarget(id), name: name,
	}, "", nil
}

// rerunAll re-runs every job of the run shown, or says why not.
func (m *Modal) rerunAll() (change, string, tea.Cmd) {
	r, notice := m.doneRun()
	if notice != "" || !m.hasRun {
		return change{}, notice, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActRerun, nil); refused {
		return change{}, "", cmd
	}
	name := jobview.RunName(r)
	svc, repo, id := m.svc, m.repo, r.ID
	return change{
		ask: ui.Confirm{
			Question: "Re-run all jobs of " + name + "?",
			Run:      m.send("re-run "+name, func() *optimistic.Op { return svc.RerunRun(repo, id) }),
		},
		target: runTarget(id), name: name,
	}, "", nil
}

// rerunJob re-runs the job under the cursor of the jobs, or says why not.
func (m *Modal) rerunJob() (change, string, tea.Cmd) {
	r, notice := m.doneRun()
	if notice != "" || !m.hasRun {
		return change{}, notice, nil
	}
	j, ok := m.jobs.selected()
	if !ok || m.jobs.runID != r.ID {
		return change{}, "Pick a job to re-run.", nil
	}
	if m.focus == jobsPane && m.jobs.onGroup() {
		return change{}, "Pick a job of the group to re-run.", nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActRerun, nil); refused {
		return change{}, "", cmd
	}
	name, job := jobview.RunName(r), ui.OneLine(j.Name)
	svc, repo, runID, jobID := m.svc, m.repo, r.ID, j.ID
	return change{
		ask: ui.Confirm{
			Question: "Re-run " + job + " of " + name + "?",
			Run:      m.send("re-run "+job, func() *optimistic.Op { return svc.RerunJob(repo, runID, jobID) }),
		},
		target: runTarget(runID) + "/" + strconv.FormatInt(jobID, 10), name: job + " of " + name,
	}, "", nil
}

// cancelRun cancels the run shown, or says why not.
func (m *Modal) cancelRun() (change, string, tea.Cmd) {
	if !m.hasRun {
		return change{}, "", nil
	}
	r := m.run
	name := jobview.RunName(r)
	if r.Done() || r.Status == core.RunCancelling {
		return change{}, name + " isn't running.", nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActCancelRun, nil); refused {
		return change{}, "", cmd
	}
	svc, repo, id := m.svc, m.repo, r.ID
	return change{
		ask: ui.Confirm{
			Question: "Cancel " + name + "?",
			Run:      m.send("cancel "+name, func() *optimistic.Op { return svc.CancelRun(repo, id) }),
		},
		target: runTarget(id), name: name,
	}, "", nil
}

// runTarget tells run id apart from the others in a change.
func runTarget(id int64) string { return strconv.FormatInt(id, 10) }

// gate decides what the viewer may do in the repository.
func (m *Modal) gate() ui.Gate {
	return ui.Gate{Repo: m.repo, Caps: m.caps}
}

// doneRun returns the run shown, if it completed, as a re-run needs, or
// the notice that says it is still running.
func (m *Modal) doneRun() (r core.Run, notice string) {
	if !m.hasRun {
		return core.Run{}, ""
	}
	if !m.run.Done() {
		notice = jobview.RunName(m.run) + " is still running."
		if k := m.keys.Cancel.Help().Key; k != "" {
			notice += " " + k + " cancels it."
		}
		return core.Run{}, notice
	}
	return m.run, ""
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
