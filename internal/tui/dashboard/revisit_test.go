package dashboard

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// expire marks what went past its TTL in svc, as another screen was on
// view, and forgets the calls so far.
func (f *fakeService) expire(what ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range what {
		f.expired[w] = true
	}
	f.calls = nil
}

func (f *fakeService) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(slices.Values(f.calls))
}

// Coming back reads again what went past its TTL, once, and nothing that
// is still fresh; the dashboard shows what it had until the new values
// arrive.
func TestRevisitReadsWhatWentStale(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	s.Blur()
	svc.expire("header", "work", "@me@")
	svc.mu.Lock()
	svc.work.ReviewRequested.Items[0].Issue.Title = "Render the cells anew"
	svc.mu.Unlock()
	s.Focus()
	cmd := s.Revisit()
	if cmd == nil {
		t.Fatal("Revisit read nothing, want what went stale")
	}
	if v := screen(s); !strings.Contains(v, "Render only the cells that changed") || !strings.Contains(v, "updating…") {
		t.Errorf("before the new values arrive the dashboard should show the old ones, updating:\n%s", v)
	}
	run(t, s, cmd)
	if got, want := svc.called(), []string{"header", "repos @me@", "work"}; !slices.Equal(got, want) {
		t.Errorf("read %v again, want %v", got, want)
	}
	if v := screen(s); !strings.Contains(v, "Render the cells anew") || strings.Contains(v, "updating…") {
		t.Errorf("the new values should show once they arrive:\n%s", v)
	}
	// All is fresh now.
	svc.expire()
	if cmd := s.Revisit(); cmd != nil {
		run(t, s, cmd)
	}
	if got := svc.called(); len(got) != 0 {
		t.Errorf("read %v with everything fresh, want nothing", got)
	}
}

// A kept value an earlier session served stale is read past, in one read.
func TestRevisitReadsPastWhatWasKept(t *testing.T) {
	svc := newFake()
	svc.stale = true
	s := newSection(t, svc, nil, 140, 38)
	s.Blur()
	svc.expire("header")
	s.Focus()
	run(t, s, s.Revisit())
	if got := svc.called(); !slices.Equal(got, []string{"header"}) {
		t.Errorf("read %v, want the header once", got)
	}
	if s.header.value.Stale || s.updating() {
		t.Error("the header is still stale")
	}
}

// A dashboard not started yet reads everything as it starts instead.
func TestRevisitBeforeStart(t *testing.T) {
	svc := newFake()
	s := New(t.Context(), svc, nil, WithNow(func() time.Time { return now }))
	if cmd := s.Revisit(); cmd != nil {
		t.Error("Revisit before Init read something")
	}
}

// When only the repositories went stale, the profile says they are read
// again until they arrive.
func TestRevisitOfTheRepositoriesShowsUpdating(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	s.Blur()
	svc.expire("@me@")
	s.Focus()
	cmd := s.Revisit()
	if cmd == nil {
		t.Fatal("Revisit read nothing, want the repositories")
	}
	if v := screen(s); !strings.Contains(v, "updating…") || !strings.Contains(v, "repo-000") {
		t.Errorf("the repositories should show, updating, until the new ones arrive:\n%s", v)
	}
	run(t, s, cmd)
	if got := svc.called(); !slices.Equal(got, []string{"repos @me@"}) {
		t.Errorf("read %v again, want the repositories", got)
	}
	if v := screen(s); strings.Contains(v, "updating…") {
		t.Errorf("still updating once the repositories arrived:\n%s", v)
	}
}

// Coming back reads the inbox and the repository of the directory again
// once they went past their TTL, as the other panes are, and not while
// they are fresh.
func TestRevisitReadsInboxAndHere(t *testing.T) {
	other := core.RepoRef{Owner: "someone", Name: "elsewhere"}
	stars := 9
	repos := &fakeRepos{get: func(_ context.Context, r core.RepoRef) (core.Repo, error) {
		return core.Repo{Ref: r, Description: "Read for its card", Stars: stars}, nil
	}}
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38, WithHere(other, repos))
	lists, gets := in.lists, repos.reads()

	s.Blur()
	s.Focus()
	if cmd := s.Revisit(); cmd != nil {
		run(t, s, cmd)
	}
	if in.lists != lists || repos.reads() != gets {
		t.Errorf("read the inbox %d and the repository %d more times while fresh, want none", in.lists-lists, repos.reads()-gets)
	}

	s.Blur()
	in.mu.Lock()
	in.expired = true
	in.mu.Unlock()
	repos.expire()
	stars = 10
	s.Focus()
	run(t, s, s.Revisit())
	if in.lists != lists+1 || repos.reads() != gets+1 {
		t.Errorf("read the inbox %d and the repository %d more times once stale, want once each", in.lists-lists, repos.reads()-gets)
	}
	if c, _ := s.pinned.selected(); c.repo.Stars != 10 {
		t.Errorf("the card here has %d stars, want the 10 read again", c.repo.Stars)
	}
}
