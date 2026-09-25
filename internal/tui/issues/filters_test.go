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

func TestPrefetchFilters(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	started(t, svc, 80, 30, WithFilterPrefetch())
	want := firstPages(core.FilterOpen, core.FilterClosed, core.FilterAll)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want the list shown, then the other filters: %v", got, want)
	}
}

func TestPrefetchFiltersSkipsCached(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.pages[issuesvc.ListQuery{Repo: testRepo, State: core.FilterAll}] = core.Page[core.Issue]{}
	started(t, svc, 80, 30, WithFilterPrefetch())
	want := firstPages(core.FilterOpen, core.FilterClosed)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want %v", got, want)
	}
}

func TestPrefetchedFilterShowsWithoutRequest(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	n := len(svc.requested())
	press(t, h, "f")
	if it, ok := h.list.Item(0); !ok || it.Number != 996 {
		t.Errorf("closed filter starts with %v, %v; want #996", it.Number, ok)
	}
	if got := ansi.Strip(h.View()); !strings.Contains(got, "996") {
		t.Errorf("closed filter shows\n%s", got)
	}
	press(t, h, "f")
	if got := h.list.Len(); got != 12 {
		t.Errorf("all filter shows %d issues, want 12", got)
	}
	press(t, h, "f")
	if got := svc.requested(); len(got) != n {
		t.Errorf("switching filters requested %v, want nothing", got[n:])
	}
}

func TestPrefetchFiltersReadsNoDetailsAhead(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithFilterPrefetch(), WithPrefetch(5, time.Millisecond))
	for _, n := range svc.getCalls() {
		if n == 996 || n == 991 {
			t.Errorf("read closed issue #%d ahead of its filter", n)
		}
	}
	press(t, h, "f")
	if got := svc.getCalls(); !slices.Contains(got, 996) {
		t.Errorf("read issues %v, want the closed ones once shown", got)
	}
}

func TestPrefetchFiltersCancelledByRepo(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	svc.mu.Lock()
	ctxs := slices.Clone(svc.listCtxs[1:])
	svc.mu.Unlock()
	if len(ctxs) != 2 {
		t.Fatalf("read %d filters ahead, want 2", len(ctxs))
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
	svc.stateErrs = map[core.StateFilter]error{core.FilterClosed: &core.RateLimitError{Reset: testNow}}
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	want := firstPages(core.FilterOpen, core.FilterClosed)
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want nothing after the rate limit: %v", got, want)
	}
	// Moving about reads nothing more ahead.
	press(t, h, "down", "up")
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want nothing more", got)
	}

	// Another repository, and back, reads ahead again.
	svc.mu.Lock()
	svc.stateErrs = nil
	svc.mu.Unlock()
	run(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	run(t, h, h.Update(ui.RepoMsg{Repo: testRepo}))
	want = append(want, firstPages(core.FilterClosed, core.FilterAll)...)
	got := slices.DeleteFunc(svc.requested(), func(q issuesvc.ListQuery) bool { return q.Repo != testRepo })
	if !slices.Equal(got, want) {
		t.Errorf("requested %v, want the filters not read yet: %v", got, want)
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
