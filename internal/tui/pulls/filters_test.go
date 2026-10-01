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

func TestPrefetchFiltersAfterTheFirstSwitch(t *testing.T) {
	closed := pulls.ListQuery{Repo: repo, State: core.StateClosed}
	tests := []struct {
		name  string
		fresh []pulls.ListQuery
		keys  []string
		want  []pulls.ListQuery
	}{{
		name: "no switch",
		want: firstPages(repo, core.StateOpen),
	}, {
		name: "next tab",
		keys: []string{"]"},
		// The closed ones are the user's; the merged ones are read ahead.
		want: firstPages(repo, core.StateOpen, core.StateClosed, core.StateMerged),
	}, {
		name: "previous tab",
		keys: []string{"["},
		want: firstPages(repo, core.StateOpen, "", core.StateClosed, core.StateMerged),
	}, {
		name:  "next tab, the rest cached",
		fresh: []pulls.ListQuery{closed},
		keys:  []string{"]", "]"},
		want:  firstPages(repo, core.StateOpen, core.StateMerged),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			for _, q := range tt.fresh {
				svc.fresh[q] = true
			}
			h := started(t, svc, 80, 30, readingTabs())
			for _, k := range tt.keys {
				press(t, h, k)
			}
			if got := svc.requested(); !slices.Equal(got, tt.want) {
				t.Errorf("requested %v, want %v", got, tt.want)
			}
		})
	}
}

// switched starts the section on repo with the other filters read ahead
// after a first switch to the closed ones, and back.
func switched(t *testing.T, svc *fakeService, opts ...Option) *host {
	t.Helper()
	h := started(t, svc, 80, 30, append([]Option{readingTabs()}, opts...)...)
	press(t, h, "]")
	press(t, h, "[")
	return h
}

func TestPrefetchedFilterShowsWithoutRequest(t *testing.T) {
	svc := newFakeService()
	h := switched(t, svc)
	n := len(svc.requested())
	press(t, h, "]")
	if got := screen(h); !strings.Contains(got, "Drop the unused REST client") {
		t.Errorf("closed tab shows\n%s", got)
	}
	press(t, h, "]")
	if got := screen(h); !strings.Contains(got, "Rename the watch package") {
		t.Errorf("merged tab shows\n%s", got)
	}
	press(t, h, "[")
	press(t, h, "[")
	if got := svc.requested(); len(got) != n {
		t.Errorf("switching filters requested %v, want nothing", got[n:])
	}
}

func TestPrefetchFiltersReadsNoDetailsAhead(t *testing.T) {
	svc := newFakeService()
	h := switched(t, svc, readingAhead(4, time.Millisecond, true))
	for _, n := range svc.got() {
		if pr := svc.find(n); pr.State == core.StateMerged {
			t.Errorf("read the detail of #%d, %s, ahead of its filter", n, pr.State)
		}
	}
	press(t, h, "]")
	if got := svc.got(); !slices.Contains(got, 93) {
		t.Errorf("read details %v, want the closed ones once shown", got)
	}
}

func TestPrefetchFiltersOncePerRepository(t *testing.T) {
	svc := newFakeService()
	h := switched(t, svc)
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	drain(t, h, h.Update(ui.RepoMsg{Repo: other}))
	n := len(svc.requested())
	// Back again, with the pages forgotten: they aren't read ahead again,
	// even after another switch.
	svc.mu.Lock()
	clear(svc.fresh)
	svc.mu.Unlock()
	drain(t, h, h.Update(ui.RepoMsg{Repo: repo}))
	press(t, h, "]")
	want := firstPages(repo, core.StateOpen, core.StateClosed)
	if got := svc.requested()[n:]; !slices.Equal(got, want) {
		t.Errorf("requested %v on coming back, want only the lists shown: %v", got, want)
	}
}

func TestPrefetchFiltersCancelledByRepo(t *testing.T) {
	svc := newFakeService()
	svc.mu.Lock()
	svc.fresh[pulls.ListQuery{Repo: repo, State: core.StateOpen}] = true
	svc.mu.Unlock()
	h := started(t, svc, 80, 30, readingTabs())
	press(t, h, "]")
	svc.mu.Lock()
	ctxs := slices.Clone(svc.listCtxs[1:])
	svc.mu.Unlock()
	if len(ctxs) != 1 {
		t.Fatalf("read %d filters ahead, want 1", len(ctxs))
	}
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	drain(t, h, h.Update(ui.RepoMsg{Repo: other}))
	for _, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Error("a read ahead of the old repository wasn't cancelled")
		}
	}
	// The other repository, on the same tab, waits for a switch of its
	// own.
	want := firstPages(other, core.StateClosed)
	if got := svc.requested()[2:]; !slices.Equal(got, want) {
		t.Errorf("requested %v for another repository, want %v", got, want)
	}
}

func TestPrefetchFiltersStopsAtRateLimit(t *testing.T) {
	svc := newFakeService()
	svc.stateErrs = map[core.State]error{core.StateMerged: &core.RateLimitError{Reset: clock}}
	h := started(t, svc, 80, 30, readingTabs())
	press(t, h, "[")
	want := firstPages(repo, core.StateOpen, "", core.StateClosed, core.StateMerged)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want nothing after the rate limit: %v", got, want)
	}
}

func TestNoFilterPrefetchByDefault(t *testing.T) {
	svc := newFakeService()
	started(t, svc, 80, 30, readingAhead(4, time.Millisecond, false))
	want := firstPages(repo, core.StateOpen)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v without WithFilterPrefetch, want %v", got, want)
	}
}
