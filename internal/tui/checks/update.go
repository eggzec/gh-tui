package checks

import (
	"errors"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

var errNoJob = errors.New("the job of this check isn't in its run")

// Update handles the step's keys and reads, and passes the rest to the job
// view. Messages of other steps are ignored.
func (s *Step) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return s.press(msg)
	case checksMsg:
		if msg.id != s.id {
			return nil
		}
		return s.receive(msg)
	case jobMsg:
		if msg.id != s.id {
			return nil
		}
		return s.receiveJob(msg)
	case tickMsg:
		if msg.id != s.id {
			return nil
		}
		return s.ticked()
	case ui.SyncMsg:
		return s.synced(msg)
	case ui.OnlineMsg:
		return s.online()
	case ui.CapsMsg:
		if msg.Repo.Same(s.q.Repo) {
			s.opts.caps = msg.Caps
		}
		return nil
	case ui.DoneMsg:
		if msg.From != Title {
			return nil
		}
		return s.done(msg)
	case ui.ReopenedMsg:
		if s.opts.ret == nil || msg.Modal != s.opts.ret {
			return nil
		}
		return s.resume()
	case logview.CloseMsg:
		if msg.ID == s.view.LogID() {
			s.back()
		}
		return nil
	case spinner.TickMsg:
		if msg.ID == s.spin.ID() {
			return s.spun(msg)
		}
	}
	var cmd tea.Cmd
	s.view, cmd = s.view.Update(msg)
	return cmd
}

// checksMsg carries the checks of the pull request.
type checksMsg struct {
	id     int64
	checks core.Checks
	err    error
}

// read reads the checks, which costs a GraphQL query unless they are
// cached fresh.
func (s *Step) read() tea.Cmd {
	s.loading = true
	svc, ctx, id, q := s.svc, s.ctx, s.id, s.q
	cmd := func() tea.Msg {
		ctx, end := obs.Begin(ctx, "checks.list")
		c, err := svc.Checks(ctx, q)
		end(err, "span", "tui", "repo", q.Repo.String(), "number", q.Number, "checks", len(c.Runs)+len(c.Statuses))
		return checksMsg{id: id, checks: c, err: err}
	}
	if s.loaded {
		return cmd
	}
	return tea.Batch(cmd, s.startSpinner())
}

func (s *Step) receive(msg checksMsg) tea.Cmd {
	s.loading = false
	if msg.err != nil {
		if s.ctx.Err() == nil {
			s.err = msg.err
		}
		return nil
	}
	s.setChecks(msg.checks)
	s.syncWatch()
	return s.startTick()
}

// press handles a key: the confirmation takes the answer, a search of the
// log every key, the step's own keys come next, and then those of what it
// shows.
func (s *Step) press(msg tea.KeyPressMsg) tea.Cmd {
	s.notice = ""
	if s.ask != nil {
		return s.answer(msg)
	}
	if s.mode == jobMode && s.view.Capturing() {
		return s.updateView(msg)
	}
	k := s.keys
	switch {
	case key.Matches(msg, k.Back):
		if s.mode == jobMode && s.view.Query() != "" {
			// The back key clears the search first.
			return s.updateView(msg)
		}
		if s.mode == listMode {
			id := s.id
			return func() tea.Msg { return CloseMsg{ID: id} }
		}
		s.back()
		return nil
	// ctrl+r re-runs, though refresh holds it too.
	case key.Matches(msg, k.RerunFailed):
		if r, ok := s.current(); ok && r.job() {
			if cmd, refused := s.gate().Refuse(ui.ActRerun, nil); refused {
				return cmd
			}
		}
		s.askRerun()
		return nil
	case key.Matches(msg, k.Refresh):
		return s.refresh()
	case key.Matches(msg, k.Open):
		return s.open()
	}
	switch s.mode {
	case jobMode:
		return s.pressJob(msg)
	case detailMode:
		var cmd tea.Cmd
		s.detail, cmd = s.detail.Update(msg)
		return cmd
	case listMode:
	}
	return s.pressList(msg)
}

// pressList moves through the checks, and opens the one under the cursor.
func (s *Step) pressList(msg tea.KeyPressMsg) tea.Cmd {
	k := s.keys
	page := max(s.listRows()-1, 1)
	switch {
	case key.Matches(msg, k.Select):
		r, ok := s.selected()
		switch {
		case !ok:
		case r.job():
			return s.openJob(r)
		default:
			s.openDetail(r)
		}
		return nil
	case key.Matches(msg, k.Up):
		s.move(-1)
	case key.Matches(msg, k.Down):
		s.move(1)
	case key.Matches(msg, k.PageUp):
		s.move(-page)
	case key.Matches(msg, k.PageDown):
		s.move(page)
	case key.Matches(msg, k.Home):
		s.move(-len(s.rows))
	case key.Matches(msg, k.End):
		s.move(len(s.rows))
	}
	return nil
}

// pressJob passes a key to the job view. A preview of the file of an
// annotation hides the step, which stops its polls until it is back.
func (s *Step) pressJob(msg tea.KeyPressMsg) tea.Cmd {
	opens := s.view.OnAnnotations() && key.Matches(msg, s.keys.Select)
	cmd := s.updateView(msg)
	if opens && cmd != nil {
		s.pause()
	}
	return cmd
}

func (s *Step) updateView(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.view, cmd = s.view.Update(msg)
	return cmd
}

// back steps back from a job or a detail to the checks.
func (s *Step) back() {
	s.mode, s.check = listMode, row{}
	s.job = jobState{}
	s.view.Clear()
	s.view.Blur()
	s.unfollow()
	s.layout()
}

// refresh reads the checks again, or what failed to load of the job shown.
func (s *Step) refresh() tea.Cmd {
	if s.mode == jobMode {
		if s.job.err != nil {
			return s.retryJob()
		}
		return s.view.Retry()
	}
	// GraphQL has no validators, so the checks are read again in full.
	s.svc.Invalidate(s.q.Repo)
	s.err = nil
	return s.read()
}

// retryJob reads again the job shown, which failed to load.
func (s *Step) retryJob() tea.Cmd {
	s.job.err, s.job.loading = nil, !s.job.hasJob
	return tea.Batch(s.readJob(), s.startSpinner())
}

// online reads again, now that GitHub answers again, what failed for want
// of an answer from it: the checks while none show, and the job shown and
// its log. The checks shown are the poll's to bring up to date.
func (s *Step) online() tea.Cmd {
	var cmds []tea.Cmd
	if !s.loaded && !s.loading && ui.Unreached(s.err) {
		s.err = nil
		cmds = append(cmds, s.read())
	}
	if s.mode == jobMode {
		if ui.Unreached(s.job.err) {
			cmds = append(cmds, s.retryJob())
		}
		cmds = append(cmds, ui.RetryUnreached(&s.view))
	}
	return tea.Batch(cmds...)
}

// open opens the job, or the check shown or under the cursor, on GitHub or
// where its app points.
func (s *Step) open() tea.Cmd {
	if s.mode == jobMode && s.job.hasJob && s.job.job.URL != "" {
		return ui.Open(s.job.job.URL)
	}
	if r, ok := s.current(); ok && r.url() != "" {
		return ui.Open(r.url())
	}
	return nil
}

// synced takes a sync event: the checks moved, as a poll found and
// cached, or the run of the job shown did.
func (s *Step) synced(msg ui.SyncMsg) tea.Cmd {
	if msg.Err != nil || s.hidden {
		return nil
	}
	switch msg.Key {
	case actionssvc.ChecksSyncKey(s.q):
		if c, ok := s.svc.CachedChecks(s.q); ok {
			s.setChecks(c)
		}
		s.syncWatch()
		return s.startTick()
	case actionssvc.RunSyncKey(s.q.Repo, s.following):
		if s.following == 0 {
			return nil
		}
		return s.jobFromCache()
	}
	return nil
}

// jobFromCache shows the job as the cache has it after a change or a
// poll, or reads it when a new attempt's jobs aren't there yet.
func (s *Step) jobFromCache() tea.Cmd {
	if s.mode != jobMode {
		return nil
	}
	show, ok := s.fromCacheJob()
	if !ok {
		return tea.Batch(s.readJob(), s.startTick())
	}
	return tea.Batch(show, s.startTick())
}

// refreshShown takes the check shown from the checks read again.
func (s *Step) refreshShown() {
	want := s.check.key()
	for _, r := range s.rows {
		if r.key() == want {
			s.check = r
			break
		}
	}
	s.rendered = ""
	s.layout()
}

// syncWatch has the sync engine poll the checks while some are pending and
// the step shows, and stops it otherwise.
func (s *Step) syncWatch() {
	if s.opts.watch == nil {
		return
	}
	if s.hidden || !s.loaded || !s.checks.Pending() {
		s.unwatch()
		return
	}
	if s.stopWatch == nil {
		s.stopWatch = s.opts.watch(s.q)
	}
}

// unwatch stops polling the checks.
func (s *Step) unwatch() {
	if s.stopWatch != nil {
		s.stopWatch()
	}
	s.stopWatch = nil
}

// pause stops the polls while a preview hides the step: its messages go
// to the preview.
func (s *Step) pause() {
	s.hidden = true
	s.unwatch()
	s.unfollow()
	s.view.Pause()
}

// Hide stops the polls while another modal is open in place of the one
// the step is in, until that modal is reopened.
func (s *Step) Hide() { s.pause() }

// resume starts again what stopped while the step was hidden, from what
// the cache has now.
func (s *Step) resume() tea.Cmd {
	s.hidden, s.ticking, s.spinning = false, false, false
	if c, ok := s.svc.CachedChecks(s.q); ok {
		s.setChecks(c)
	}
	s.syncWatch()
	cmds := []tea.Cmd{s.startTick()}
	if s.mode == jobMode {
		show, _ := s.fromCacheJob()
		cmds = append(cmds, show)
		s.follow()
	}
	if s.loadingAny() {
		cmds = append(cmds, s.startSpinner())
	}
	return tea.Batch(cmds...)
}

// spun spins the spinner while something loads.
func (s *Step) spun(msg spinner.TickMsg) tea.Cmd {
	if !s.loadingAny() {
		s.spinning = false
		return nil
	}
	var cmd tea.Cmd
	s.spin, cmd = s.spin.Update(msg)
	return cmd
}

// tickMsg moves the timers of what runs on.
type tickMsg struct {
	id int64
}

// startTick starts the timers while the step shows something that runs,
// unless they run.
func (s *Step) startTick() tea.Cmd {
	if s.ticking || s.hidden || s.opts.tick <= 0 || !s.running() {
		return nil
	}
	s.ticking = true
	return s.tick()
}

func (s *Step) tick() tea.Cmd {
	id := s.id
	return tea.Tick(s.opts.tick, func(time.Time) tea.Msg { return tickMsg{id: id} })
}

// ticked moves the timers on, until nothing shown runs.
func (s *Step) ticked() tea.Cmd {
	if s.hidden || !s.running() {
		s.ticking = false
		return nil
	}
	return s.tick()
}

// running reports whether a check, or the job shown, runs.
func (s *Step) running() bool {
	if s.mode == jobMode && s.job.hasJob && !s.job.job.Done() {
		return true
	}
	return s.loaded && s.checks.Pending()
}
