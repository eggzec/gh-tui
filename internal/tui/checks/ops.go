package checks

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// askRerun asks to re-run the failed jobs of the run of the check shown,
// or of the one under the cursor, once the run completed. When there is
// none to re-run, the notice may say why. By the time the user says yes,
// the run may have been re-run elsewhere or its jobs read again, so it
// asks again, and re-runs only the same run with the same question.
func (s *Step) askRerun() {
	c, id, name, notice := s.rerun()
	if c.Question == "" {
		s.notice = notice
		return
	}
	ask := ui.Recheck(c.Question, name, func() (ui.Confirm, bool, tea.Cmd) {
		if cmd, refused := s.gate().Refuse(ui.ActRerun, nil); refused {
			return ui.Confirm{}, false, cmd
		}
		again, againID, _, _ := s.rerun()
		return again, again.Question != "" && againID == id, nil
	})
	s.ask = &ask
}

// rerun returns re-running the failed jobs of the run of the check shown,
// or of the one under the cursor, as a question, with the ID and the name
// of the run. The question is empty when there is nothing to re-run, and
// notice may say why.
func (s *Step) rerun() (c ui.Confirm, id int64, name, notice string) {
	r, ok := s.current()
	if !ok || !r.job() {
		return ui.Confirm{}, 0, "", ""
	}
	run, ok := s.job.run, s.mode == jobMode && s.job.hasRun
	if !ok {
		run, ok = s.svc.CachedRun(s.q.Repo, r.check.RunID)
	}
	if !ok {
		return ui.Confirm{}, 0, "", "Open the check to re-run its jobs."
	}
	name = jobview.RunName(run)
	if !run.Done() {
		return ui.Confirm{}, 0, "", name + " is still running."
	}
	jobs := "the failed jobs"
	if p, ok := s.svc.CachedAllJobs(actionssvc.JobsQuery{Repo: s.q.Repo, RunID: run.ID, Attempt: run.Attempt}); ok {
		switch n := jobview.FailedJobs(p.Items); n {
		case 0:
			return ui.Confirm{}, 0, "", name + " has no failed jobs to re-run."
		case 1:
			jobs = "1 failed job"
		default:
			jobs = strconv.Itoa(n) + " failed jobs"
		}
	}
	svc, repo, id := s.svc, s.q.Repo, run.ID
	return ui.Confirm{
		Question: "Re-run " + jobs + " of " + name + "?",
		Run: func() tea.Cmd {
			op := svc.RerunFailedJobs(repo, id)
			return tea.Batch(s.jobFromCache(), ui.Do(s.parent, Title, ui.Refused(op), "re-run the failed jobs of "+name))
		},
	}, id, name, ""
}

// answer takes the answer to the confirmation: yes makes the change and
// sends it, and no drops it. Other keys leave the question open.
func (s *Step) answer(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := s.keys.Confirm.Answer(*s.ask, msg)
	if done {
		s.ask = nil
	}
	return cmd
}

// done re-renders from the cache once a re-run was confirmed or rolled
// back. A confirmed re-run starts new check runs, so the checks are read
// again, and the run of the job shown too, which the service left stale.
func (s *Step) done(msg ui.DoneMsg) tea.Cmd {
	cmd := s.jobFromCache()
	if msg.Err != nil {
		return cmd
	}
	s.svc.Invalidate(s.q.Repo)
	cmds := []tea.Cmd{cmd, s.read()}
	if s.mode == jobMode && s.following == 0 {
		cmds = append(cmds, s.readJob())
	}
	return tea.Batch(cmds...)
}
