package dashboard

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
)

// kind names one of the dashboard's reads.
type kind int

const (
	kindHeader kind = iota
	kindWork
	kindContributions
	kindInbox
	kindHere
)

// loadedMsg carries the value of one read, for the dashboard with id, in
// the refresh gen.
type loadedMsg struct {
	id    int64
	gen   int
	kind  kind
	value any
	err   error
}

// load reads everything the dashboard shows. Each read serves what the
// cache has at once, so a fresh cache costs no request.
func (s *Section) load() tea.Cmd {
	cmds := []tea.Cmd{s.readHeader(false), s.readWork(false), s.readContributions(false), s.readInbox(false)}
	if s.hasHere() && !s.hereRepo.ok {
		cmds = append(cmds, s.readHere())
	}
	return tea.Batch(cmds...)
}

func (s *Section) readHeader(again bool) tea.Cmd {
	s.header.loading = true
	return readCmd(s, kindHeader, "dashboard.header", func(ctx context.Context) (core.Header, served, error) {
		h, err := s.svc.Header(ctx, dashboard.HeaderQuery{Again: again})
		return h, served{h.Stale, h.Offline, h.Limited}, err
	})
}

func (s *Section) readWork(again bool) tea.Cmd {
	s.work.loading = true
	return readCmd(s, kindWork, "dashboard.work", func(ctx context.Context) (core.Work, served, error) {
		w, err := s.svc.Work(ctx, dashboard.WorkQuery{Again: again})
		return w, served{w.Stale, w.Offline, w.Limited}, err
	})
}

func (s *Section) readContributions(again bool) tea.Cmd {
	s.contribs.loading = true
	return readCmd(s, kindContributions, "dashboard.contributions", func(ctx context.Context) (core.Contributions, served, error) {
		c, err := s.svc.Contributions(ctx, dashboard.ContributionsQuery{Again: again})
		return c, served{c.Stale, c.Offline, c.Limited}, err
	})
}

func (s *Section) readInbox(again bool) tea.Cmd {
	// The pane says why the token may not read the inbox instead.
	if s.inbox == nil || s.voice.Token.Check(core.NeedNotifications) != nil {
		return nil
	}
	s.notes.loading = true
	in := s.inbox
	return readCmd(s, kindInbox, "dashboard.inbox", func(ctx context.Context) (core.Page[core.Notification], served, error) {
		p, err := in.List(ctx, notifications.ListQuery{Again: again})
		return p, served{p.Stale, p.Offline, p.Limited}, err
	})
}

// hasHere reports whether the dashboard reads the repository of the
// current directory.
func (s *Section) hasHere() bool {
	return s.here != (core.RepoRef{}) && s.hereRepos != nil
}

func (s *Section) readHere() tea.Cmd {
	s.hereRepo.loading = true
	repos, ref := s.hereRepos, s.here
	return readCmd(s, kindHere, "dashboard.here", func(ctx context.Context) (core.Repo, served, error) {
		r, err := repos.Get(ctx, ref)
		return r, served{}, err
	})
}

// served says how a read came by its value: kept by an earlier session,
// or read earlier, because GitHub couldn't be reached or rate limited it.
type served struct {
	stale, offline, limited bool
}

// readCmd runs read in a command, as a trace of its own named name.
func readCmd[V any](s *Section, k kind, name string, read func(ctx context.Context) (V, served, error)) tea.Cmd {
	ctx, id, gen := s.ctx, s.id, s.gen
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, name)
		v, how, err := read(ctx)
		end(err, "span", "tui", "stale", how.stale, "offline", how.offline, "limited", how.limited)
		return loadedMsg{id: id, gen: gen, kind: k, value: v, err: err}
	}
}

// loaded takes the value of a read, and reads it again if it was kept by an
// earlier session.
func (s *Section) loaded(msg loadedMsg) tea.Cmd {
	if msg.id != s.id || msg.gen != s.gen {
		return nil
	}
	switch msg.kind {
	case kindHeader:
		if !take(&s.header, msg) {
			return nil
		}
		s.setHeader()
		if s.header.value.Stale {
			return s.readHeader(true)
		}
		// The organizations are known now, so their tabs are too.
		return s.repos.start()
	case kindWork:
		if !take(&s.work, msg) {
			return nil
		}
		s.tasks.set(s.work.value)
		if s.work.value.Stale {
			return s.readWork(true)
		}
	case kindContributions:
		if !take(&s.contribs, msg) {
			return nil
		}
		s.setContributions()
		if s.contribs.value.Stale {
			return s.readContributions(true)
		}
	case kindInbox:
		if !take(&s.notes, msg) {
			return nil
		}
		s.setInbox()
		if s.notes.value.Stale {
			return s.readInbox(true)
		}
	case kindHere:
		if take(&s.hereRepo, msg) {
			s.pinned.hereRepo = s.hereRepo.value
			s.setPinned(s.header.value.Pinned)
		}
	}
	return nil
}

// take records the outcome of a read in r, and reports whether it brought
// a value. A failed read keeps the value shown before.
func take[V any](r *read[V], msg loadedMsg) bool {
	r.loading = false
	r.err = msg.err
	if msg.err != nil {
		return false
	}
	v, ok := msg.value.(V)
	if !ok {
		return false
	}
	r.value, r.ok = v, true
	return true
}

// refresh reads everything again from GitHub.
func (s *Section) refresh() tea.Cmd {
	s.svc.Invalidate()
	s.opener.Resume()
	s.ahead.Resume()
	s.aheadRepos.Resume()
	s.aheadPinned.Resume()
	s.gen++
	s.hereRepo.ok = false
	return tea.Batch(s.load(), s.repos.reload())
}

// online reads again, now that GitHub answers again, what failed for want
// of an answer from it, or was served from what an earlier read kept
// while GitHub couldn't be reached or rate limited it:
// the profile, the work, the calendar, the inbox, the repository of the
// directory and the lists of repositories. What is being read already
// is left to finish.
func (s *Section) online(msg ui.OnlineMsg) tea.Cmd {
	// A rate limit is the token's, and has lifted unless one holds.
	if !msg.Limited {
		s.opener.Resume()
		s.ahead.Resume()
		s.aheadRepos.Resume()
		s.aheadPinned.Resume()
	}
	if !s.started {
		return nil
	}
	var cmds []tea.Cmd
	if unreached(s.header, s.header.value.Offline || s.header.value.Limited) {
		cmds = append(cmds, s.readHeader(true))
	}
	if unreached(s.work, s.work.value.Offline || s.work.value.Limited) {
		cmds = append(cmds, s.readWork(true))
	}
	if unreached(s.contribs, s.contribs.value.Offline || s.contribs.value.Limited) {
		cmds = append(cmds, s.readContributions(true))
	}
	if unreached(s.notes, s.notes.value.Offline || s.notes.value.Limited) {
		cmds = append(cmds, s.readInbox(true))
	}
	if unreached(s.hereRepo, false) && s.hasHere() {
		cmds = append(cmds, s.readHere())
	}
	cmds = append(cmds, s.repos.online())
	return tea.Batch(cmds...)
}

// unreached reports whether r, which isn't being read, failed for want of
// an answer from GitHub, or shows a value kept, served while GitHub
// couldn't be reached or rate limited the read.
func unreached[V any](r read[V], kept bool) bool {
	return !r.loading && (ui.Unreached(r.err) || r.ok && r.err == nil && kept)
}

// Revisit reads again what went past its TTL while another screen was on
// view: the profile, the work, the calendar, the inbox, the repository of
// the directory and the lists of repositories read. The dashboard shows
// what it has until the new values arrive, and what is still fresh isn't
// read.
func (s *Section) Revisit() tea.Cmd {
	if !s.started {
		return nil
	}
	// Each read sets Again: what is shown may be a value an earlier
	// session kept, served stale, and this read must reach GitHub anyway.
	var cmds []tea.Cmd
	if !s.header.loading && !s.svc.FreshHeader() {
		cmds = append(cmds, s.readHeader(true))
	}
	if !s.work.loading && !s.svc.FreshWork(dashboard.WorkQuery{}) {
		cmds = append(cmds, s.readWork(true))
	}
	if !s.contribs.loading && !s.svc.FreshContributions() {
		cmds = append(cmds, s.readContributions(true))
	}
	// The polls keep the inbox current, but may not have while another
	// screen was on view.
	if s.inbox != nil && !s.notes.loading && !s.inbox.FreshList(notifications.ListQuery{}) {
		cmds = append(cmds, s.readInbox(true))
	}
	if s.hasHere() && !s.hereRepo.loading && !s.hereRepos.FreshGet(s.here) {
		cmds = append(cmds, s.readHere())
	}
	cmds = append(cmds, s.repos.revisit())
	cmd := tea.Batch(cmds...)
	if cmd != nil {
		// What is being read again shows as updating.
		s.render()
	}
	return cmd
}

func (s *Section) setHeader() {
	h := s.header.value
	s.setPinned(h.Pinned)
	s.repos.setOrgs(h.Orgs, h.Profile.Login)
}

func (s *Section) setContributions() {
	c := s.contribs.value
	weeks := make([][]calendar.Day, len(c.Weeks))
	for i, w := range c.Weeks {
		weeks[i] = make([]calendar.Day, len(w))
		for j, d := range w {
			weeks[i][j] = calendar.Day{Date: d.Date, Count: d.Count, Level: d.Level}
		}
	}
	period := "the last year"
	if s.calDays > 0 {
		period = "the last " + strconv.Itoa(s.calDays) + " days"
	}
	s.cal.SetEmptyText("No contributions in " + period + ".")
	s.cal.SetWeeks(weeks)
	// The total GitHub reports is for the year, and a range counts its
	// own days instead.
	s.cal.SetTotal(c.Total)
	// The calendar is as wide as its range and total need.
	s.layout()
}

// updating reports whether something shown is being read again, such as
// what an earlier session kept, or what went stale while another screen
// was on view.
func (s *Section) updating() bool {
	return s.header.ok && s.header.loading ||
		s.work.ok && s.work.loading ||
		s.contribs.ok && s.contribs.loading ||
		s.repos.reloading()
}

// offlineNow reports whether anything shown was served because GitHub
// couldn't be reached.
func (s *Section) offlineNow() bool {
	return s.header.ok && s.header.value.Offline ||
		s.work.ok && s.work.value.Offline ||
		s.contribs.ok && s.contribs.value.Offline ||
		s.notes.ok && s.notes.value.Offline
}

// limitedNow reports whether anything shown was served because GitHub rate
// limited the read.
func (s *Section) limitedNow() bool {
	return s.header.ok && s.header.value.Limited ||
		s.work.ok && s.work.value.Limited ||
		s.contribs.ok && s.contribs.value.Limited ||
		s.notes.ok && s.notes.value.Limited
}
