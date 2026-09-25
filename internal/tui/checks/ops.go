package checks

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// confirm is a re-run that waits for the user to say yes, on the last
// line of the step.
type confirm struct {
	// question asks, such as "Re-run 2 failed jobs of CI #4812?", and what
	// names the change for the error toast.
	question, what string
	start          func() *optimistic.Op
}

// askRerun asks to re-run the failed jobs of the run of the check shown,
// or of the one under the cursor, once the run completed.
func (s *Step) askRerun() {
	r, ok := s.current()
	if !ok || !r.job() {
		return
	}
	run, ok := s.job.run, s.mode == jobMode && s.job.hasRun
	if !ok {
		run, ok = s.svc.CachedRun(s.q.Repo, r.check.RunID)
	}
	if !ok {
		s.notice = "Open the check to re-run its jobs."
		return
	}
	name := jobview.RunName(run)
	if !run.Done() {
		s.notice = name + " is still running."
		return
	}
	jobs := "the failed jobs"
	if p, ok := s.svc.CachedJobs(actionssvc.JobsQuery{Repo: s.q.Repo, RunID: run.ID, Attempt: run.Attempt}); ok {
		switch n := jobview.FailedJobs(p.Items); n {
		case 0:
			s.notice = name + " has no failed jobs to re-run."
			return
		case 1:
			jobs = "1 failed job"
		default:
			jobs = strconv.Itoa(n) + " failed jobs"
		}
	}
	svc, repo, id := s.svc, s.q.Repo, run.ID
	s.ask = &confirm{
		question: "Re-run " + jobs + " of " + name + "?",
		what:     "re-run the failed jobs of " + name,
		start:    func() *optimistic.Op { return svc.RerunFailedJobs(repo, id) },
	}
}

// answer takes the answer to the confirmation: yes makes the change and
// sends it, and anything else drops it.
func (s *Step) answer(msg tea.KeyPressMsg) tea.Cmd {
	ask := s.ask
	s.ask = nil
	if !key.Matches(msg, s.keys.Yes) {
		return nil
	}
	op := ask.start()
	return tea.Batch(s.jobFromCache(), ui.Do(s.parent, Title, ui.Refused(op), ask.what))
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
