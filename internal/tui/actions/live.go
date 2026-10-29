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

// liveRuns lists the runs to keep up to date: the run shown while it hasn't
// completed, and every run of the feed that is open, has its jobs loaded and
// hasn't completed. A folded run shows no jobs, so it isn't polled.
func (m *Modal) liveRuns() []int64 {
	var ids []int64
	if m.hasRun && !m.run.Done() {
		ids = append(ids, m.run.ID)
	}
	open := m.runs.UnfoldedItems()
	for i := range open {
		r := m.current(open[i])
		if j := m.jobsOf[r.ID]; j != nil && j.loaded && !r.Done() && !slices.Contains(ids, r.ID) {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// follow has the sync engine follow the runs that liveRuns lists, and stop
// following those that no longer are.
func (m *Modal) follow() {
	if m.opts.follow == nil {
		return
	}
	want := m.liveRuns()
	for id, stop := range m.follows {
		if !slices.Contains(want, id) {
			stop()
			delete(m.follows, id)
		}
	}
	for _, id := range want {
		if _, ok := m.follows[id]; ok {
			continue
		}
		if m.follows == nil {
			m.follows = map[int64]func(){}
		}
		m.follows[id] = m.opts.follow(m.repo, id)
	}
}

// unfollow stops following every run.
func (m *Modal) unfollow() {
	for id, stop := range m.follows {
		stop()
		delete(m.follows, id)
	}
}

// followed returns the run that the sync event key is of, if one is followed.
func (m *Modal) followed(key string) (int64, bool) {
	for id := range m.follows {
		if key == actionssvc.RunSyncKey(m.repo, id) {
			return id, true
		}
	}
	return 0, false
}

// synced takes a sync event: the runs changed, as the revalidator found,
// or the run followed did, as a poll found and cached.
func (m *Modal) synced(msg ui.SyncMsg) tea.Cmd {
	if msg.Err != nil {
		return nil
	}
	if msg.Key == actionssvc.SyncKey(m.repo) {
		return m.runs.Reload()
	}
	if id, ok := m.followed(msg.Key); ok {
		return m.refreshRun(id)
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
	return m.refreshRun(m.run.ID)
}

// refreshRun shows run id and its jobs as the cache has them, whether the run is
// the one shown or an open one.
func (m *Modal) refreshRun(id int64) tea.Cmd {
	r, ok := m.svc.CachedRun(m.repo, id)
	if ok {
		m.live[id] = r
	}
	shown := m.hasRun && id == m.run.ID
	switch {
	case shown && ok:
		m.run = r
	case shown:
		r = m.run
	case !ok:
		return nil
	}
	j := m.jobsOf[id]
	if j == nil {
		return nil
	}
	if r.Attempt != j.attempt {
		// The jobs of the attempt before stay until those of the new one
		// arrive, which keep the cursor on the job of the same name.
		prev := *j
		j = m.entry(r)
		j.items, j.loaded = prev.items, prev.loaded
		j.tree, j.lines, j.folds = prev.tree, prev.lines, prev.folds
		m.runs.Resort()
	}
	var cmd tea.Cmd
	q := actionssvc.JobsQuery{Repo: m.repo, RunID: id, Attempt: r.Attempt}
	if p, ok := m.svc.CachedAllJobs(q); ok {
		cmd = m.setJobs(j, p)
	} else {
		cmd = m.readJobs(r)
	}
	m.follow()
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

// running reports whether the run shown, one of its jobs, or an open run
// whose jobs are loaded runs.
func (m *Modal) running() bool {
	if !m.hasRun {
		return false
	}
	return !m.run.Done() || slices.ContainsFunc(m.cur().items, func(j core.Job) bool { return !j.Done() }) ||
		len(m.liveRuns()) > 0
}
