package checks

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
)

// jobState is the job of the check shown, and its run.
type jobState struct {
	run    core.Run
	hasRun bool
	job    core.Job
	hasJob bool
	// loading is set while the job is read with none to show yet, and err
	// once the read failed.
	loading bool
	err     error
}

// openJob shows the job of the check r: from memory at once if it is
// there, and read, which the service answers from memory once the run
// completed.
func (s *Step) openJob(r row) tea.Cmd {
	s.mode, s.check = jobMode, r
	s.job = jobState{loading: true}
	s.view.Clear()
	s.view.Focus()
	show, _ := s.fromCacheJob()
	s.layout()
	cmds := []tea.Cmd{show, s.readJob(), s.startTick()}
	if s.job.loading {
		cmds = append(cmds, s.startSpinner())
	}
	return tea.Batch(cmds...)
}

// fromCacheJob shows the job as the cache has it, after a change or a
// poll, and reports whether it found it. A new attempt of the run has new
// jobs, so the job is found by its name there.
func (s *Step) fromCacheJob() (tea.Cmd, bool) {
	c := s.check.check
	if c == nil {
		return nil, false
	}
	run, ok := s.svc.CachedRun(s.q.Repo, c.RunID)
	if !ok {
		return nil, false
	}
	p, ok := s.svc.CachedAllJobs(actionssvc.JobsQuery{Repo: s.q.Repo, RunID: c.RunID, Attempt: run.Attempt})
	if !ok {
		return nil, false
	}
	j, ok := s.findJob(p.Items)
	if !ok {
		return nil, false
	}
	return s.setJob(run, j), true
}

// findJob finds the job of the check shown among jobs: the one it reports,
// or the one of the same name in another attempt.
func (s *Step) findJob(jobs []core.Job) (core.Job, bool) {
	c := s.check.check
	id := c.JobID
	if s.job.hasJob {
		id = s.job.job.ID
	}
	if i := slices.IndexFunc(jobs, func(j core.Job) bool { return j.ID == id }); i >= 0 {
		return jobs[i], true
	}
	name := c.Name
	if s.job.hasJob {
		name = s.job.job.Name
	}
	if i := slices.IndexFunc(jobs, func(j core.Job) bool { return j.Name == name }); i >= 0 {
		return jobs[i], true
	}
	return core.Job{}, false
}

// setJob shows job j of run, and follows the run while it runs.
func (s *Step) setJob(run core.Run, j core.Job) tea.Cmd {
	s.job = jobState{run: run, hasRun: true, job: j, hasJob: true}
	s.follow()
	c := s.check.check
	h := jobview.Hints{SHA: s.checks.SHA, NoAnnotations: j.ID == c.JobID && c.Annotations == 0}
	cmd := s.view.Show(j, false, h)
	s.layout()
	return cmd
}

// jobMsg carries the run and the job of a check.
type jobMsg struct {
	id      int64
	checkID int64
	run     core.Run
	jobs    []core.Job
	err     error
}

// readJob reads the run of the check shown, and the jobs of its attempt.
func (s *Step) readJob() tea.Cmd {
	c := s.check.check
	if c == nil {
		return nil
	}
	svc, ctx, id, repo, checkID, runID := s.svc, s.ctx, s.id, s.q.Repo, c.ID, c.RunID
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "checks.job")
		run, err := svc.Run(ctx, repo, runID)
		var p core.Page[core.Job]
		if err == nil {
			p, err = svc.AllJobs(ctx, actionssvc.JobsQuery{Repo: repo, RunID: runID, Attempt: run.Attempt})
		}
		end(err, "span", "tui", "run", runID, "jobs", len(p.Items))
		return jobMsg{id: id, checkID: checkID, run: run, jobs: p.Items, err: err}
	}
}

func (s *Step) receiveJob(msg jobMsg) tea.Cmd {
	if s.mode != jobMode || s.check.check == nil || msg.checkID != s.check.check.ID {
		return nil
	}
	s.job.loading = false
	if msg.err != nil {
		s.job.err = msg.err
		return nil
	}
	j, ok := s.findJob(msg.jobs)
	if !ok {
		s.job.err = errNoJob
		return nil
	}
	return tea.Batch(s.setJob(msg.run, j), s.startTick())
}

// follow has the sync engine follow the run of the job shown while it
// hasn't completed, and only that run, unless a preview hides the step.
func (s *Step) follow() {
	if s.opts.follow == nil {
		return
	}
	if s.hidden || s.mode != jobMode || !s.job.hasRun || s.job.run.Done() {
		s.unfollow()
		return
	}
	if s.following == s.job.run.ID {
		return
	}
	s.unfollow()
	s.following, s.stopFollow = s.job.run.ID, s.opts.follow(s.q.Repo, s.job.run.ID)
}

// unfollow stops following a run.
func (s *Step) unfollow() {
	if s.stopFollow != nil {
		s.stopFollow()
	}
	s.following, s.stopFollow = 0, nil
}
