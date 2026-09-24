package dashboard

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/service/notifications"
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
	cmds := []tea.Cmd{s.readHeader(), s.readWork(), s.readContributions(), s.readInbox()}
	if s.here != (core.RepoRef{}) && s.getHere != nil && !s.hereRepo.ok {
		cmds = append(cmds, s.readHere())
	}
	return tea.Batch(cmds...)
}

func (s *Section) readHeader() tea.Cmd {
	s.header.loading = true
	return readCmd(s, kindHeader, "dashboard.header", func(ctx context.Context) (core.Header, bool, bool, error) {
		h, err := s.svc.Header(ctx)
		return h, h.Stale, h.Offline, err
	})
}

func (s *Section) readWork() tea.Cmd {
	s.work.loading = true
	return readCmd(s, kindWork, "dashboard.work", func(ctx context.Context) (core.Work, bool, bool, error) {
		w, err := s.svc.Work(ctx, dashboard.WorkQuery{})
		return w, w.Stale, w.Offline, err
	})
}

func (s *Section) readContributions() tea.Cmd {
	s.contribs.loading = true
	return readCmd(s, kindContributions, "dashboard.contributions", func(ctx context.Context) (core.Contributions, bool, bool, error) {
		c, err := s.svc.Contributions(ctx)
		return c, c.Stale, c.Offline, err
	})
}

func (s *Section) readInbox() tea.Cmd {
	if s.inbox == nil {
		return nil
	}
	s.notes.loading = true
	in := s.inbox
	return readCmd(s, kindInbox, "dashboard.inbox", func(ctx context.Context) (core.Page[core.Notification], bool, bool, error) {
		p, err := in.List(ctx, notifications.ListQuery{})
		return p, p.Stale, p.Offline, err
	})
}

func (s *Section) readHere() tea.Cmd {
	s.hereRepo.loading = true
	get, ref := s.getHere, s.here
	return readCmd(s, kindHere, "dashboard.here", func(ctx context.Context) (core.Repo, bool, bool, error) {
		r, err := get(ctx, ref)
		return r, false, false, err
	})
}

// readCmd runs read in a command, as a trace of its own named name. A value
// that an earlier session kept is shown, and read again at once.
func readCmd[V any](s *Section, k kind, name string, read func(ctx context.Context) (v V, stale, offline bool, err error)) tea.Cmd {
	ctx, id, gen, off := s.ctx, s.id, s.gen, s.offline
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, name)
		v, stale, offline, err := read(ctx)
		end(err, "span", "tui", "stale", stale, "offline", offline)
		if offline {
			off.Mark()
		}
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
			return s.readHeader()
		}
		// The organizations are known now, so their tabs are too.
		return s.repos.start()
	case kindWork:
		if !take(&s.work, msg) {
			return nil
		}
		s.tasks.set(s.work.value)
		if s.work.value.Stale {
			return s.readWork()
		}
	case kindContributions:
		if !take(&s.contribs, msg) {
			return nil
		}
		s.setContributions()
		if s.contribs.value.Stale {
			return s.readContributions()
		}
	case kindInbox:
		if !take(&s.notes, msg) {
			return nil
		}
		if s.notes.value.Stale {
			return s.readInbox()
		}
	case kindHere:
		if take(&s.hereRepo, msg) {
			s.pinned.hereRepo = s.hereRepo.value
			s.pinned.set(s.header.value.Pinned)
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
	s.gen++
	s.hereRepo.ok = false
	return tea.Batch(s.load(), s.repos.reload())
}

func (s *Section) setHeader() {
	h := s.header.value
	s.pinned.set(h.Pinned)
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
	s.cal.SetEmptyText("No contributions in the last year.")
	s.cal.SetWeeks(weeks)
	s.cal.SetTotal(c.Total)
}

// updating reports whether something shown is being read again, such as
// what an earlier session kept.
func (s *Section) updating() bool {
	return s.header.ok && s.header.loading ||
		s.work.ok && s.work.loading ||
		s.contribs.ok && s.contribs.loading
}

// offlineNow reports whether anything shown was served because GitHub
// couldn't be reached.
func (s *Section) offlineNow() bool {
	return s.header.ok && s.header.value.Offline ||
		s.work.ok && s.work.value.Offline ||
		s.contribs.ok && s.contribs.value.Offline ||
		s.notes.ok && s.notes.value.Offline
}
