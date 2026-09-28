package actions

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// follow has the sync engine follow the run shown while it hasn't
// completed, and only that run.
func (m *Modal) follow() {
	if m.opts.follow == nil {
		return
	}
	if !m.hasRun || m.run.Done() {
		m.unfollow()
		return
	}
	if m.following == m.run.ID {
		return
	}
	m.unfollow()
	m.following, m.stop = m.run.ID, m.opts.follow(m.repo, m.run.ID)
}

// unfollow stops following a run.
func (m *Modal) unfollow() {
	if m.stop != nil {
		m.stop()
	}
	m.following, m.stop = 0, nil
}

// synced takes a sync event: the runs changed, as the revalidator found,
// or the run followed did, as a poll found and cached.
func (m *Modal) synced(msg ui.SyncMsg) tea.Cmd {
	if msg.Err != nil {
		return nil
	}
	switch msg.Key {
	case actionssvc.SyncKey(m.repo):
		return m.runs.Reload()
	case actionssvc.RunSyncKey(m.repo, m.following):
		if m.following == 0 {
			return nil
		}
		return m.fromCache()
	}
	return nil
}

// fromCache shows the run shown and its jobs as the cache has them, after
// a change or a poll: a re-run shows its jobs queued at once, and a new
// attempt its own jobs.
func (m *Modal) fromCache() tea.Cmd {
	if !m.hasRun {
		return nil
	}
	if r, ok := m.svc.CachedRun(m.repo, m.run.ID); ok {
		m.live[r.ID] = r
		m.run = r
	}
	if m.run.Attempt != m.jobs.attempt {
		// The jobs of the attempt before stay until those of the new one
		// arrive, which keep the cursor on the job of the same name.
		prev := m.jobs
		m.jobs = newJobs(m.run)
		m.jobs.items, m.jobs.cursor, m.jobs.loaded = prev.items, prev.cursor, prev.loaded
	}
	m.follow()
	var cmd tea.Cmd
	if p, ok := m.svc.CachedAllJobs(m.jobsQuery()); ok {
		cmd = m.setJobs(p, false)
	} else {
		cmd = m.readJobs()
	}
	return tea.Batch(cmd, m.startTick())
}

// runMsg carries a run read again.
type runMsg struct {
	id  int64
	run core.Run
	err error
}

// readRun reads the run shown again, such as after a change that GitHub
// accepted, which leaves it stale.
func (m *Modal) readRun() tea.Cmd {
	if !m.hasRun {
		return nil
	}
	svc, ctx, id, repo, runID := m.svc, m.ctx, m.id, m.repo, m.run.ID
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.run")
		r, err := svc.Run(ctx, repo, runID)
		end(err, "span", "tui", "run", runID, "status", string(r.Status))
		return runMsg{id: id, run: r, err: err}
	}
}

func (m *Modal) receiveRun(msg runMsg) tea.Cmd {
	if msg.err != nil || !m.hasRun || msg.run.ID != m.run.ID {
		return nil
	}
	return m.fromCache()
}

// tickMsg moves the timers of what runs on.
type tickMsg struct {
	id int64
}

// startTick starts the timers while the modal shows something that runs,
// unless they run.
func (m *Modal) startTick() tea.Cmd {
	if m.ticking || m.opts.tick <= 0 || !m.running() {
		return nil
	}
	m.ticking = true
	return m.tick()
}

func (m *Modal) tick() tea.Cmd {
	id := m.id
	return tea.Tick(m.opts.tick, func(time.Time) tea.Msg { return tickMsg{id: id} })
}

// ticked moves the timers on, until nothing shown runs.
func (m *Modal) ticked() tea.Cmd {
	if !m.running() {
		m.ticking = false
		return nil
	}
	return m.tick()
}

// running reports whether the run shown, or one of its jobs, runs.
func (m *Modal) running() bool {
	if !m.hasRun {
		return false
	}
	return !m.run.Done() || slices.ContainsFunc(m.jobs.items, func(j core.Job) bool { return !j.Done() })
}
