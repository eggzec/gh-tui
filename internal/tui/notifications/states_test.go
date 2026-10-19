package notifications

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakeStates answers States with what it holds, after counting the calls.
type fakeStates struct {
	cached map[core.SubjectRef]core.SubjectState
	asked  map[core.SubjectRef]core.SubjectState
	err    error
	calls  atomic.Int32
}

func (f *fakeStates) CachedStates([]core.Notification) map[core.SubjectRef]core.SubjectState {
	return f.cached
}

func (f *fakeStates) States(context.Context, []core.Notification) (map[core.SubjectRef]core.SubjectState, error) {
	f.calls.Add(1)
	return f.asked, f.err
}

func statesFixture(tb testing.TB, st States) (s *Section, pull, issue core.Notification) {
	tb.Helper()
	pull = core.Notification{ID: "1", Unread: true, Repo: core.RepoRef{Owner: "o", Name: "a"}, Subject: core.Subject{Type: core.SubjectPullRequest, Number: 7, Title: "p"}}
	issue = core.Notification{ID: "2", Unread: true, Repo: core.RepoRef{Owner: "o", Name: "a"}, Subject: core.Subject{Type: core.SubjectIssue, Number: 8, Title: "i"}}
	s = New(tb.Context(), newFake(pull, issue), config.Default().Keys, WithNow(func() time.Time { return now }), WithStates(st))
	s.SetTheme(ui.NewTheme(defaultPalette(), true))
	s.SetSize(100, 20)
	s.Focus()
	run(tb, s, s.Init())
	return s, pull, issue
}

func TestSectionAppliesLateStates(t *testing.T) {
	merged := core.SubjectState{State: core.StateMerged}
	pullRef := core.SubjectRef{Repo: core.RepoRef{Owner: "o", Name: "a"}, Number: 7, Type: core.SubjectPullRequest}
	f := &fakeStates{asked: map[core.SubjectRef]core.SubjectState{pullRef: merged}}
	s, pull, issue := statesFixture(t, f)
	if f.calls.Load() != 1 {
		t.Fatalf("States asked %d times, want once for the page", f.calls.Load())
	}
	if got, ok := s.State(pull); !ok || got != merged {
		t.Errorf("State(pull) = %v, %v; want %v", got, ok, merged)
	}
	if _, ok := s.State(issue); ok {
		t.Error("State(issue) known, but the answer held no state for it")
	}
}

func TestSectionShowsCachedStatesAtOnce(t *testing.T) {
	closed := core.SubjectState{State: core.StateClosed}
	issueRef := core.SubjectRef{Repo: core.RepoRef{Owner: "o", Name: "a"}, Number: 8, Type: core.SubjectIssue}
	f := &fakeStates{cached: map[core.SubjectRef]core.SubjectState{issueRef: closed}}
	s, _, issue := statesFixture(t, f)
	if got, ok := s.State(issue); !ok || got != closed {
		t.Errorf("State(issue) = %v, %v; want the cached %v", got, ok, closed)
	}
}

func TestSectionKeepsRowsWhenStatesFail(t *testing.T) {
	f := &fakeStates{err: errors.New("boom")}
	s, pull, _ := statesFixture(t, f)
	if _, ok := s.State(pull); ok {
		t.Error("State known after a failed read")
	}
	if s.feed.Len() != 2 {
		t.Errorf("feed has %d rows, want 2", s.feed.Len())
	}
}
