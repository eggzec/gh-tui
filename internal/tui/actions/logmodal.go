package actions

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// logModal is the log of a job, an overlay over the Actions modal. It
// follows the run itself while it is open, and reads its own log.
type logModal struct {
	log    jobview.Model
	parent *Modal
	keys   KeyMap
	job    core.Job

	following int64
	stop      func()
}

// openLog opens the log of job j over the modal.
func (m *Modal) openLog(j core.Job) tea.Cmd {
	m.aheadLogs.Opened(j.ID)
	m.aheadLogs.Keep(j.ID)
	o := m.opts
	lm := &logModal{parent: m, keys: m.keys, job: j}
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
	r := l.parent.ann[l.job.ID]
	return l.log.Show(l.job, rest, jobview.Hints{SHA: l.parent.run.HeadSHA, KeptNotes: r != nil && r.kept})
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

// Title names the job.
func (l *logModal) Title() string {
	return l.job.Name + l.parent.opts.icons.Separator + l.parent.repo.String()
}

// SetSize sizes the log.
func (l *logModal) SetSize(w, h int) { l.log.SetSize(w, h) }

// SetTheme styles the log.
func (l *logModal) SetTheme(t ui.Theme) { l.log.SetTheme(t) }

// View renders the log.
func (l *logModal) View() string { return l.log.View() }

// KeyContext implements ui.Hosted.
func (l *logModal) KeyContext() string { return ctxLog }

// Commands implements ui.Commanded.
func (l *logModal) Commands() []string { return []string{ui.CommandCopy} }

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
				if l.log.ClearSearch() {
					return nil
				}
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
	if pg, ok := p.svc.CachedAllJobs(p.jobsQuery()); ok {
		for i := range pg.Items {
			if pg.Items[i].ID == l.job.ID {
				l.job = pg.Items[i]
			}
		}
	}
	if p.run.Done() {
		l.unfollow()
	}
	return l.show(false)
}
