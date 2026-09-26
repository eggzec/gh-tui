package issues

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func firstPages(filters ...core.StateFilter) []issuesvc.ListQuery {
	qs := make([]issuesvc.ListQuery, 0, len(filters))
	for _, f := range filters {
		qs = append(qs, issuesvc.ListQuery{Repo: testRepo, State: f})
	}
	return qs
}

func TestPrefetchFiltersAfterTheFirstSwitch(t *testing.T) {
	tests := []struct {
		name   string
		cached []core.StateFilter
		keys   []string
		want   []issuesvc.ListQuery
	}{{
		name: "no switch",
		want: firstPages(core.FilterOpen),
	}, {
		name: "next tab",
		keys: []string{"]"},
		// The closed ones are the user's; all are read ahead.
		want: firstPages(core.FilterOpen, core.FilterClosed, core.FilterAll),
	}, {
		name: "previous tab",
		keys: []string{"["},
		want: firstPages(core.FilterOpen, core.FilterAll, core.FilterClosed),
	}, {
		name:   "next tab, the rest cached",
		cached: []core.StateFilter{core.FilterAll},
		keys:   []string{"]", "]"},
		want:   firstPages(core.FilterOpen, core.FilterClosed),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			for _, f := range tt.cached {
				svc.pages[issuesvc.ListQuery{Repo: testRepo, State: f}] = core.Page[core.Issue]{}
			}
			h := started(t, svc, 80, 30, WithFilterPrefetch())
			press(t, h, tt.keys...)
			if got := svc.requested(); !slices.Equal(got, tt.want) {
				t.Errorf("requested %v, want %v", got, tt.want)
			}
		})
	}
}

// switched starts the section on testRepo with the other filters read
// ahead after a first switch to the closed ones, and back.
func switched(t *testing.T, svc *fakeService, opts ...Option) *host {
	t.Helper()
	h := started(t, svc, 80, 30, append([]Option{WithFilterPrefetch()}, opts...)...)
	press(t, h, "]", "[")
	return h
}

func TestPrefetchedFilterShowsWithoutRequest(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := switched(t, svc)
	n := len(svc.requested())
	press(t, h, "]")
	if it, ok := h.list.Item(0); !ok || it.Number != 996 {
		t.Errorf("closed filter starts with %v, %v; want #996", it.Number, ok)
	}
	if got := ansi.Strip(h.View()); !strings.Contains(got, "996") {
		t.Errorf("closed filter shows\n%s", got)
	}
	press(t, h, "]")
	if got := h.list.Len(); got != 12 {
		t.Errorf("all filter shows %d issues, want 12", got)
	}
	press(t, h, "]")
	if got := svc.requested(); len(got) != n {
		t.Errorf("switching filters requested %v, want nothing", got[n:])
	}
}

func TestPrefetchFiltersReadsNoDetailsAhead(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithFilterPrefetch(), WithPrefetch(5, time.Millisecond))
	// All the issues show, and the closed ones are read ahead. #991 is
	// closed, and past the first rows of all.
	press(t, h, "[")
	if got := svc.getCalls(); slices.Contains(got, 991) {
		t.Errorf("read closed issue #991 ahead of its filter: %v", got)
	}
	press(t, h, "[")
	if got := svc.getCalls(); !slices.Contains(got, 991) {
		t.Errorf("read issues %v, want the closed ones once shown", got)
	}
}

func TestPrefetchFiltersOncePerRepository(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := switched(t, svc)
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	run(t, h, h.Update(ui.RepoMsg{Repo: other}))
	// Back again, with the pages forgotten: they aren't read ahead again,
	// even after another switch.
	svc.mu.Lock()
	clear(svc.pages)
	svc.mu.Unlock()
	n := len(svc.requested())
	run(t, h, h.Update(ui.RepoMsg{Repo: testRepo}))
	press(t, h, "]")
	want := firstPages(core.FilterOpen, core.FilterClosed)
	if got := svc.requested()[n:]; !slices.Equal(got, want) {
		t.Errorf("requested %v on coming back, want only the lists shown: %v", got, want)
	}
}

func TestPrefetchFiltersCancelledByRepo(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	press(t, h, "]")
	svc.mu.Lock()
	ctxs := slices.Clone(svc.listCtxs[2:])
	svc.mu.Unlock()
	if len(ctxs) != 1 {
		t.Fatalf("read %d filters ahead, want 1", len(ctxs))
	}
	run(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	for _, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Error("a read ahead of the old repository wasn't cancelled")
		}
	}
}

func TestPrefetchFiltersStopsAtRateLimit(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.stateErrs = map[core.StateFilter]error{core.FilterAll: &core.RateLimitError{Reset: testNow}}
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	press(t, h, "]")
	want := firstPages(core.FilterOpen, core.FilterClosed, core.FilterAll)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want nothing after the rate limit: %v", got, want)
	}
	// Moving about reads nothing more ahead.
	press(t, h, "down", "up")
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want nothing more", got)
	}
}

func TestNoFilterPrefetchByDefault(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	started(t, svc, 80, 30, WithPrefetch(5, time.Millisecond))
	want := firstPages(core.FilterOpen)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v without WithFilterPrefetch, want %v", got, want)
	}
}
