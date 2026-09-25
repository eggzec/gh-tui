package pulls

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func firstPages(r core.RepoRef, states ...core.State) []pulls.ListQuery {
	qs := make([]pulls.ListQuery, 0, len(states))
	for _, st := range states {
		qs = append(qs, pulls.ListQuery{Repo: r, State: st})
	}
	return qs
}

func TestPrefetchFilters(t *testing.T) {
	svc := newFakeService()
	started(t, svc, 80, 30, WithFilterPrefetch())
	want := firstPages(repo, core.StateOpen, core.StateClosed, core.StateMerged)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want the list shown, then the other filters: %v", got, want)
	}
}

func TestPrefetchFiltersSkipsCached(t *testing.T) {
	svc := newFakeService()
	svc.fresh[pulls.ListQuery{Repo: repo, State: core.StateClosed}] = true
	started(t, svc, 80, 30, WithFilterPrefetch())
	want := firstPages(repo, core.StateOpen, core.StateMerged)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want %v", got, want)
	}
}

func TestPrefetchedFilterShowsWithoutRequest(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	n := len(svc.requested())
	press(t, h, "f")
	if got := screen(h); !strings.Contains(got, "Drop the unused REST client") {
		t.Errorf("closed filter shows\n%s", got)
	}
	press(t, h, "f")
	if got := screen(h); !strings.Contains(got, "Rename the watch package") {
		t.Errorf("merged filter shows\n%s", got)
	}
	press(t, h, "f")
	if got := svc.requested(); len(got) != n {
		t.Errorf("switching filters requested %v, want nothing", got[n:])
	}
}

func TestPrefetchFiltersReadsNoDetailsAhead(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30, WithFilterPrefetch(), WithPrefetch(5, time.Millisecond))
	// Only the open pull requests are on screen.
	for _, n := range svc.got() {
		if pr := svc.find(n); pr.State != core.StateOpen {
			t.Errorf("read the detail of #%d, %s, ahead of its filter", n, pr.State)
		}
	}
	press(t, h, "f")
	if got := svc.got(); !slices.Contains(got, 93) {
		t.Errorf("read details %v, want the closed ones once shown", got)
	}
}

func TestPrefetchFiltersCancelledByRepo(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	svc.mu.Lock()
	ctxs := slices.Clone(svc.listCtxs[1:])
	svc.mu.Unlock()
	if len(ctxs) != 2 {
		t.Fatalf("read %d filters ahead, want 2", len(ctxs))
	}
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	drain(t, h, h.Update(ui.RepoMsg{Repo: other}))
	for _, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Error("a read ahead of the old repository wasn't cancelled")
		}
	}
	want := firstPages(other, core.StateOpen, core.StateClosed, core.StateMerged)
	if got := svc.requested()[3:]; !slices.Equal(got, want) {
		t.Errorf("requested %v for another repository, want %v", got, want)
	}
}

func TestPrefetchFiltersStopsAtRateLimit(t *testing.T) {
	svc := newFakeService()
	svc.stateErrs = map[core.State]error{core.StateClosed: &core.RateLimitError{Reset: clock}}
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	want := firstPages(repo, core.StateOpen, core.StateClosed)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want nothing after the rate limit: %v", got, want)
	}
	// Switching back and forth reads nothing more ahead.
	press(t, h, "f")
	press(t, h, "f")
	press(t, h, "f")
	want = append(want, firstPages(repo, core.StateClosed, core.StateMerged)...)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want only the lists switched to: %v", got, want)
	}

	// Another repository reads ahead again.
	svc.mu.Lock()
	svc.stateErrs = nil
	svc.mu.Unlock()
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	drain(t, h, h.Update(ui.RepoMsg{Repo: other}))
	want = firstPages(other, core.StateOpen, core.StateClosed, core.StateMerged)
	if got := svc.requested()[4:]; !slices.Equal(got, want) {
		t.Errorf("requested %v for another repository, want %v", got, want)
	}
}

func TestNoFilterPrefetchByDefault(t *testing.T) {
	svc := newFakeService()
	started(t, svc, 80, 30, WithPrefetch(5, time.Millisecond))
	want := firstPages(repo, core.StateOpen)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v without WithFilterPrefetch, want %v", got, want)
	}
}
