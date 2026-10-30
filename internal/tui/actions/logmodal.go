package actions

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// logModal is the log of a job, an overlay over the Actions modal. It
// follows the run itself while it is open, and reads its own log.
type logModal struct {
	log    jobview.Model
	parent *Modal
	keys   KeyMap
	job    core.Job
	hints  jobview.Hints

	following int64
	stop      func()

	// attempts holds the job in each attempt of its run, the first one
	// first, and at the one shown. nth is the place of the job among the
	// jobs of its name in an attempt, for a matrix whose jobs share one.
	attempts []attemptLog
	at, nth  int
	// reading is the attempt whose jobs are being read for a step.
	reading int
}

// attemptLog is the job of the log in one attempt of the run, once it is
// known, and where its log was left.
type attemptLog struct {
	attempt int
	job     core.Job
	// found is set once the job is known, and missing once the attempt is
	// known to have none of its name.
	found, missing bool
	// pos is where the log was left, if seen.
	pos  pager.Position
	seen bool
}

// attemptJobsMsg carries the jobs of an attempt, read to step the log to.
type attemptJobsMsg struct {
	log     *logModal
	attempt int
	delta   int
	page    core.Page[core.Job]
	err     error
}

// openLog opens the log of job j over the modal.
func (m *Modal) openLog(j core.Job, h jobview.Hints) tea.Cmd {
	m.aheadLogs.Opened(j.ID)
	m.aheadLogs.Keep(j.ID)
	o := m.opts
	lm := &logModal{parent: m, keys: m.keys, job: j, hints: h}
	lm.attempts, lm.at = m.attemptsOf(j)
	if items, ok := lm.cachedJobs(lm.attempts[lm.at].attempt); ok {
		lm.nth = place(items, j)
	}
	lm.log = jobview.New(m.ctx, m.svc, m.repo, m.keys.job(),
		jobview.WithIcons(o.icons), jobview.WithRest(o.prefetch.logs.Rest),
		jobview.WithClock(o.now), jobview.WithVoice(*o.voice))
	lm.log.SetTheme(m.theme)
	lm.log.SetSize(m.width, m.height)
	lm.log.Focus()
	lm.follow()
	// The log's messages go to the open modal only, so it is shown once open.
	return tea.Sequence(ui.OpenModalOver(lm, m), lm.show(true))
}

func (l *logModal) show(rest bool) tea.Cmd {
	return l.log.Show(l.job, rest, l.hints)
}

func (l *logModal) follow() {
	p := l.parent
	if p.opts.follow == nil || !p.hasRun || p.run.Done() || l.following == p.run.ID {
		return
	}
	l.following, l.stop = p.run.ID, p.opts.follow(p.repo, p.run.ID)
}

func (l *logModal) unfollow() {
	if l.stop != nil {
		l.stop()
	}
	l.following, l.stop = 0, nil
}

// Title names the job, after its attempt while the run has several.
func (l *logModal) Title() string {
	name := l.job.Name
	if n := len(l.attempts); n > 1 {
		name = fmt.Sprintf("%d/%d attempt %d  %s", l.at+1, n, l.attempts[l.at].attempt, name)
	}
	return name + l.parent.opts.icons.Separator + l.parent.repo.String()
}

// SetSize sizes the log.
func (l *logModal) SetSize(w, h int) { l.log.SetSize(w, h) }

// SetTheme styles the log.
func (l *logModal) SetTheme(t ui.Theme) { l.log.SetTheme(t) }

// View renders the log.
func (l *logModal) View() string { return l.log.View() }

// KeyContext implements ui.Hosted.
func (l *logModal) KeyContext() string { return ctxLog }

// Commands implements ui.Commanded: copy, and the steps through the
// attempts of the run while it has several.
func (l *logModal) Commands() []string {
	if len(l.attempts) < 2 {
		return []string{ui.CommandCopy}
	}
	return []string{ui.CommandCopy, ui.CommandNextFile, ui.CommandPrevFile}
}

// Step implements ui.Stepping: it shows the job in the attempt delta
// places on among those that have it, stopping at the ends. An attempt
// whose jobs aren't known yet is read first, and the step taken once they
// are.
func (l *logModal) Step(delta int) tea.Cmd {
	if delta == 0 || len(l.attempts) < 2 {
		return nil
	}
	dir, left := 1, delta
	if delta < 0 {
		dir, left = -1, -delta
	}
	to := -1
	for i := l.at + dir; i >= 0 && i < len(l.attempts); i += dir {
		e := &l.attempts[i]
		if !e.found && !e.missing {
			items, ok := l.cachedJobs(e.attempt)
			if !ok {
				return l.readAttempt(e.attempt, delta)
			}
			l.match(e, items)
		}
		if e.missing {
			continue
		}
		to = i
		left--
		if left == 0 {
			break
		}
	}
	if to < 0 {
		return nil
	}
	return l.stepTo(to)
}

// stepTo shows the job of attempts[i], where its log was left.
func (l *logModal) stepTo(i int) tea.Cmd {
	cur := &l.attempts[l.at]
	cur.job, cur.pos, cur.seen = l.job, l.log.Position(), true
	l.at = i
	e := l.attempts[i]
	l.job = e.job
	l.parent.aheadLogs.Opened(e.job.ID)
	l.parent.aheadLogs.Keep(e.job.ID)
	cmd := l.show(false)
	if e.seen {
		l.log.SetPosition(e.pos)
	}
	return cmd
}

// match finds the job of the log in items, the jobs of the attempt of e,
// by its name and its place among the jobs of that name.
func (l *logModal) match(e *attemptLog, items []core.Job) {
	k := 0
	for i := range items {
		if items[i].Name != l.job.Name {
			continue
		}
		if k == l.nth {
			e.job, e.found = items[i], true
			return
		}
		k++
	}
	e.missing = true
}

// place returns the place of j among the jobs of its name in items.
func place(items []core.Job, j core.Job) int {
	k := 0
	for i := range items {
		if items[i].ID == j.ID {
			return k
		}
		if items[i].Name == j.Name {
			k++
		}
	}
	return 0
}

// attemptsQuery reads the jobs of attempt of the run of the log.
func (l *logModal) attemptsQuery(attempt int) actionssvc.JobsQuery {
	return actionssvc.JobsQuery{Repo: l.parent.repo, RunID: l.job.RunID, Attempt: attempt}
}

// cachedJobs returns the jobs of attempt, if the cache has them.
func (l *logModal) cachedJobs(attempt int) ([]core.Job, bool) {
	pg, ok := l.parent.svc.CachedAllJobs(l.attemptsQuery(attempt))
	return pg.Items, ok
}

// readAttempt reads the jobs of attempt, to step delta once they are in.
func (l *logModal) readAttempt(attempt, delta int) tea.Cmd {
	if l.reading == attempt {
		return nil
	}
	l.reading = attempt
	svc, ctx, q := l.parent.svc, l.parent.ctx, l.attemptsQuery(attempt)
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.jobs")
		p, err := svc.AllJobs(ctx, q)
		end(err, "span", "tui", "run", q.RunID, "attempt", q.Attempt, "jobs", len(p.Items))
		return attemptJobsMsg{log: l, attempt: attempt, delta: delta, page: p, err: err}
	}
}

// receiveAttempt takes the jobs of an attempt read for a step, and steps.
// A read that failed leaves the attempt to be read on the next step.
func (l *logModal) receiveAttempt(msg attemptJobsMsg) tea.Cmd {
	if msg.log != l || msg.attempt != l.reading {
		return nil
	}
	l.reading = 0
	if msg.err != nil {
		return nil
	}
	for i := range l.attempts {
		if e := &l.attempts[i]; e.attempt == msg.attempt && !e.found {
			l.match(e, msg.page.Items)
		}
	}
	return l.Step(msg.delta)
}

// attemptsOf lists the attempts of the run of j, with the index of the
// attempt of j: every attempt of the run shown, or only that of j.
func (m *Modal) attemptsOf(j core.Job) (list []attemptLog, at int) {
	a, n := j.Attempt, 0
	if m.hasRun && m.run.ID == j.RunID {
		n = m.run.Attempt
	}
	if a == 0 {
		a = n
	}
	if n < 2 || a < 1 || a > n {
		return []attemptLog{{attempt: a, job: j, found: true}}, 0
	}
	list = make([]attemptLog, n)
	for i := range list {
		list[i].attempt = i + 1
	}
	list[a-1].job, list[a-1].found = j, true
	return list, a - 1
}

// Selected implements ui.Selector.
func (l *logModal) Selected() (ui.Selection, bool) {
	p := l.parent
	return ui.Selection{What: "job", URL: l.job.URL, Repo: p.repo, SHA: p.run.HeadSHA, Owner: p.repo.Owner}, true
}

// KeyLayers implements ui.Keyed.
func (l *logModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{l.log.Layer()}
}

// Hide stops following the run and the poll of the log.
func (l *logModal) Hide() {
	l.unfollow()
	l.log.Pause()
}

// Discard ends the log for good.
func (l *logModal) Discard() {
	l.unfollow()
	l.log.Clear()
}

// Update takes the keys that close the log, and passes everything else to
// the job view, whose messages are scoped to it.
func (l *logModal) Update(msg tea.Msg) tea.Cmd {
	k := l.keys
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if !l.log.Capturing() {
			switch {
			case keymap.Matches(msg, k.Dismiss):
				return l.close()
			case keymap.Matches(msg, k.Back):
				return l.close()
			case keymap.Matches(msg, k.Refresh):
				return l.log.Retry()
			case keymap.Matches(msg, k.Open) && l.job.URL != "":
				return ui.Open(l.job.URL)
			case keymap.Matches(msg, k.ViewFile):
				return l.parent.viewWorkflow()
			}
		}
	case ui.SyncMsg:
		return l.synced(msg)
	case attemptJobsMsg:
		return l.receiveAttempt(msg)
	case ui.OnlineMsg:
		return ui.RetryUnreached(&l.log)
	case ui.ReopenedMsg:
		if msg.Modal != ui.Modal(l) {
			return nil
		}
		l.follow()
		return tea.Batch(l.show(false), l.log.ReadLost())
	}
	var cmd tea.Cmd
	l.log, cmd = l.log.Update(msg)
	return cmd
}

// close discards the log and returns to the modal under it.
func (l *logModal) close() tea.Cmd {
	l.Discard()
	return ui.LeaveModal(l)
}

// synced shows the job as the cache has it after a poll of its run.
func (l *logModal) synced(msg ui.SyncMsg) tea.Cmd {
	p := l.parent
	if msg.Err != nil || l.following == 0 || msg.Key != actionssvc.RunSyncKey(p.repo, l.following) {
		return nil
	}
	if r, ok := p.svc.CachedRun(p.repo, l.following); ok {
		p.run = r
		p.live[r.ID] = r
	}
	// Only the latest attempt runs; the older ones are done.
	last := len(l.attempts) - 1
	q, job := p.jobsQuery(), &l.job
	if last > 0 {
		q = l.attemptsQuery(l.attempts[last].attempt)
	}
	if l.at != last {
		job = &l.attempts[last].job
	}
	if pg, ok := p.svc.CachedAllJobs(q); ok && job.ID != 0 {
		for i := range pg.Items {
			if pg.Items[i].ID == job.ID {
				*job = pg.Items[i]
			}
		}
	}
	if p.run.Done() {
		l.unfollow()
	}
	if l.at != last {
		return nil
	}
	return l.show(false)
}
