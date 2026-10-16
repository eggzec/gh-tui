package pulls

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/visits"
)

// memVisitStore is a VisitStore in memory that counts its saves.
type memVisitStore struct {
	mu    sync.Mutex
	kept  map[string]visits.State
	saves int
	err   error
	// saveFails is how many of the next saves fail, without keeping anything.
	saveFails int
}

func (s *memVisitStore) Load(key string) (visits.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kept[key], s.err
}

func (s *memVisitStore) Save(key string, st visits.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveFails > 0 {
		s.saveFails--
		return errors.New("disk full")
	}
	if s.kept == nil {
		s.kept = map[string]visits.State{}
	}
	s.kept[key] = st
	s.saves++
	return nil
}

func (s *memVisitStore) state() (st visits.State, saves int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kept[visits.Key(repo.String(), 142)], s.saves
}

// visiting returns the modal of #142, opened at clock on its Overview, over a
// store that holds prev, and the function that moves the modal's clock.
func visiting(t *testing.T, store *memVisitStore, comments []core.Comment) (*host, *detailModal, func(time.Duration)) {
	t.Helper()
	var ns atomic.Int64
	ns.Store(clock.UnixNano())
	svc := newFakeService()
	svc.thread = comments
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	h := started(t, svc, 100, 60, WithVisits(store), WithClock(func() time.Time { return time.Unix(0, ns.Load()) }))
	h.SetTheme(theme(true))
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter opened no pull request")
	}
	return h, m, func(d time.Duration) { ns.Add(int64(d)) }
}

func TestVisitUnderThreeSecondsWritesNothing(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	_, m, advance := visiting(t, store, nil)
	advance(2900 * time.Millisecond)
	m.Discard()
	if _, n := store.state(); n != 0 {
		t.Errorf("saves = %d, want none", n)
	}
}

func TestVisitCountsOverviewAndFilesTogether(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{kept: map[string]visits.State{
		visits.Key(repo.String(), 142): {
			LastSeen: visits.LastSeen{Head: "old", At: clock.Add(-time.Hour)},
		},
	}}
	h, m, advance := visiting(t, store, nil)
	advance(1500 * time.Millisecond)
	drain(t, h, m.switchTo(filesTab))
	advance(1500 * time.Millisecond)
	m.Discard()
	got, n := store.state()
	if n != 1 {
		t.Fatalf("saves = %d, want 1", n)
	}
	if got.LastSeen.Head != "a1b2c3d" || !got.LastSeen.At.Equal(clock) {
		t.Errorf("LastSeen = %+v, want head a1b2c3d opened at %v", got.LastSeen, clock)
	}
}

func TestVisitDoesNotCountTheTimeTheModalIsHidden(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	_, m, advance := visiting(t, store, nil)
	advance(2 * time.Second)
	m.Hide()
	advance(time.Minute)
	_ = m.Update(ui.ReopenedMsg{Modal: m})
	advance(500 * time.Millisecond)
	m.Discard()
	if _, n := store.state(); n != 0 {
		t.Fatalf("saves = %d after 2.5 s shown and a minute hidden, want none", n)
	}

	store = &memVisitStore{}
	_, m, advance = visiting(t, store, nil)
	advance(2 * time.Second)
	m.Hide()
	advance(time.Minute)
	_ = m.Update(ui.ReopenedMsg{Modal: m})
	advance(1500 * time.Millisecond)
	m.Discard()
	if _, n := store.state(); n != 1 {
		t.Errorf("saves = %d after 3.5 s shown, want 1", n)
	}
}

func TestVisitNeverCountsTheConversation(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	h, m, advance := visiting(t, store, nil)
	drain(t, h, m.switchTo(conversationTab))
	advance(time.Minute)
	m.Discard()
	if _, n := store.state(); n != 0 {
		t.Errorf("saves = %d, want none", n)
	}
}

func TestVisitIsWrittenWhenTheAppQuits(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{}
	_, m, advance := visiting(t, store, nil)
	advance(3 * time.Second)
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if _, n := store.state(); n != 1 {
		t.Errorf("saves = %d, want 1", n)
	}
}

func TestVisitIsNotWrittenOverAnUnreadState(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{err: errors.New("permission denied")}
	_, m, advance := visiting(t, store, nil)
	advance(time.Minute)
	m.Discard()
	if _, n := store.state(); n != 0 {
		t.Errorf("saves = %d, want none", n)
	}
}

// newLines returns the lines of the conversation that name who and are tagged
// new.
func newLines(m *detailModal) (tagged, all map[string]bool) {
	tagged, all = map[string]bool{}, map[string]bool{}
	for l := range strings.SplitSeq(ansi.Strip(m.View()), "\n") {
		for _, who := range []string{"alice", "bob", "me"} {
			if strings.Contains(l, who+" ·") {
				all[who] = true
				tagged[who] = strings.Contains(l, " new")
			}
		}
	}
	return tagged, all
}

func visitedComments() []core.Comment {
	return []core.Comment{
		{ID: "1", Author: core.User{Login: "bob"}, Body: "old", CreatedAt: clock.Add(-3 * time.Hour)},
		{ID: "2", Author: core.User{Login: "alice"}, Body: "fresh", CreatedAt: clock.Add(-time.Hour)},
		{ID: "3", Author: core.User{Login: "me"}, Body: "mine", CreatedAt: clock.Add(-30 * time.Minute)},
	}
}

func TestOnlyOthersCommentsSinceTheLastVisitAreNew(t *testing.T) {
	t.Parallel()
	store := &memVisitStore{kept: map[string]visits.State{
		visits.Key(repo.String(), 142): {LastSeen: visits.LastSeen{Head: "x", At: clock.Add(-2 * time.Hour)}},
	}}
	h, m, _ := visiting(t, store, visitedComments())
	drain(t, h, m.switchTo(conversationTab))
	m.viewer = "me"
	m.redrawNew()
	tagged, all := newLines(m)
	if len(all) != 3 {
		t.Fatalf("the conversation shows %v, want bob, alice and me:\n%s", all, ansi.Strip(m.View()))
	}
	if tagged["bob"] || !tagged["alice"] || tagged["me"] {
		t.Errorf("new = %v, want only alice", tagged)
	}
}

func TestNothingIsNewWithoutAPreviousVisit(t *testing.T) {
	t.Parallel()
	h, m, _ := visiting(t, &memVisitStore{}, visitedComments())
	drain(t, h, m.switchTo(conversationTab))
	m.viewer = "me"
	m.redrawNew()
	tagged, all := newLines(m)
	if len(all) != 3 {
		t.Fatalf("the conversation shows %v, want 3 authors", all)
	}
	for who, isNew := range tagged {
		if isNew {
			t.Errorf("%s is tagged new with no previous visit", who)
		}
	}
}

func TestReviewsSinceTheLastVisitAreNew(t *testing.T) {
	t.Parallel()
	m := &detailModal{viewer: "me"}
	m.visit.prev.LastSeen.At = clock.Add(-time.Hour)
	for _, c := range []struct {
		at    time.Time
		login string
		want  bool
	}{
		{clock.Add(-time.Minute), "alice", true},
		{clock.Add(-time.Minute), "ME", false},
		{clock.Add(-2 * time.Hour), "alice", false},
		{clock.Add(-time.Hour), "alice", false},
	} {
		if got := m.isNew(c.at, c.login); got != c.want {
			t.Errorf("isNew(%v, %q) = %v, want %v", c.at, c.login, got, c.want)
		}
	}
	m.viewer = ""
	if m.isNew(clock, "alice") {
		t.Error("something is new before the viewer is known")
	}
}
