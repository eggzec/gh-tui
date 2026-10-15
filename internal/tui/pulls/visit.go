package pulls

import (
	"errors"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/visits"
)

// The modal remembers when the user last looked at a pull request, and marks
// what arrived since as new. A look counts once the Overview or the Files tab
// was shown for a few seconds in all, so that opening a pull request by
// accident, or only to read its conversation or checks, doesn't clear what is
// new.

// VisitStore keeps what the user saw of pull requests, by key. Its calls do
// I/O.
type VisitStore interface {
	Load(key string) (visits.State, error)
	Save(key string, state visits.State) error
}

// visitAfter is how long the Overview and the Files tabs must have shown, in
// all, for the visit to count.
const visitAfter = 3 * time.Second

// freshWord is the tag of an entry that arrived since the last visit.
const freshWord = "new"

// memVisits is the visits of one session, for when none are kept on disk.
type memVisits struct {
	mu   sync.Mutex
	kept map[string]visits.State
}

func (s *memVisits) Load(key string) (visits.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.kept[key]
	st.Viewed = maps.Clone(st.Viewed)
	return st, nil
}

func (s *memVisits) Save(key string, state visits.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kept == nil {
		s.kept = map[string]visits.State{}
	}
	state.Viewed = maps.Clone(state.Viewed)
	s.kept[key] = state
	return nil
}

// visitState is what the modal knows of the visits to its pull request.
type visitState struct {
	store VisitStore
	// opened is when the modal opened, which is what a visit that counts keeps
	// as the time of the last look.
	opened time.Time
	// prev is the state that was kept when the modal opened: what was new since
	// is measured from its time. It is empty until loaded is set.
	prev   visits.State
	loaded bool
	// readFailed is set when the state could not be read for a reason that says
	// nothing of its content, and may still hold something: nothing is saved
	// over it.
	readFailed bool
	// watched is how long the tabs that count showed before the stretch that
	// began at since, which is running while on is set.
	watched time.Duration
	since   time.Time
	on      bool
	// done is set once the modal closed, so that a late save writes nothing.
	done bool
}

// visitLoadedMsg carries the state read from disk to the modal that asked.
type visitLoadedMsg struct {
	owner *detailModal
	state visits.State
	err   error
}

// countsAsVisit reports whether time on tab t counts toward a visit.
func countsAsVisit(t modalTab) bool { return t == overviewTab || t == filesTab }

// visitKey returns the key of the visits to this pull request.
func (m *detailModal) visitKey() string { return visits.Key(m.repo.String(), m.number) }

// loadVisit reads the visits to the pull request from disk.
func (m *detailModal) loadVisit() tea.Cmd {
	store, key := m.visit.store, m.visitKey()
	if store == nil {
		return nil
	}
	return func() tea.Msg {
		st, err := store.Load(key)
		return visitLoadedMsg{owner: m, state: st, err: err}
	}
}

// receiveVisit takes the state that was read, and marks what is new since.
func (m *detailModal) receiveVisit(msg visitLoadedMsg) {
	if msg.owner != m {
		return
	}
	v := &m.visit
	v.loaded = true
	if msg.err != nil {
		slog.WarnContext(m.ctx, "visits not read", "span", "tui", "err", ui.ShortPath(msg.err.Error()))
		v.readFailed = !errors.Is(msg.err, visits.ErrDamaged)
		return
	}
	v.prev = msg.state
	m.redrawNew()
}

// redrawNew draws the entries again, once what makes some of them new is known.
func (m *detailModal) redrawNew() {
	if !m.visit.prev.LastSeen.At.IsZero() {
		m.thread.Redraw()
	}
}

// startVisit begins the count of the time on the tabs that count, on the tab
// the modal opens on.
func (m *detailModal) startVisit() {
	v := &m.visit
	v.opened = m.now()
	v.tick(v.opened, countsAsVisit(m.tab))
}

// tick ends the stretch that was running, and starts another if on is set.
func (v *visitState) tick(now time.Time, on bool) {
	if v.on {
		v.watched += now.Sub(v.since)
	}
	v.since, v.on = now, on
}

// shown returns how long the tabs that count have shown, up to now.
func (v *visitState) shown(now time.Time) time.Duration {
	if v.on {
		return v.watched + now.Sub(v.since)
	}
	return v.watched
}

// writeVisit keeps this visit, if it counts: the head that shows now, and the
// time the modal opened. What was kept of the files viewed stays. It writes at
// once, as the files are small and the modal may be gone before a command
// would run.
func (m *detailModal) writeVisit() error {
	v := &m.visit
	if v.store == nil || v.done || !v.loaded || v.readFailed || v.shown(m.now()) < visitAfter {
		return nil
	}
	return v.store.Save(m.visitKey(), visits.State{
		LastSeen: visits.LastSeen{Head: m.detail.HeadSHA, At: v.opened},
		Viewed:   maps.Clone(v.prev.Viewed),
	})
}

// isNew reports whether something that login did at time at arrived since the
// last visit. What the viewer did is never new, and when the viewer isn't known
// yet nothing is, so that their own words don't show as new for a moment.
func (m *detailModal) isNew(at time.Time, login string) bool {
	prev := m.visit.prev.LastSeen.At
	if prev.IsZero() || !at.After(prev) || m.viewer == "" {
		return false
	}
	return !strings.EqualFold(login, m.viewer)
}

// newTag returns the tag that follows the age of an entry that is new, or "".
// Unstyled, it is the text alone, for what measures widths.
func (m *detailModal) newTag(at time.Time, login string, styled bool) string {
	if !m.isNew(at, login) {
		return ""
	}
	if !styled {
		return " " + freshWord
	}
	return " " + m.st.fresh.Render(freshWord)
}
