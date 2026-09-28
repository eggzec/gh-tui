package issues

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// The section is what the app's filter modal filters.
var (
	_ ui.Filterable = (*Section)(nil)
	_ ui.Chipper    = (*Section)(nil)
	_ ui.Claimer    = (*Section)(nil)
)

// apply applies query in the filter form of h, as the modal does when the
// user types it and presses enter.
func apply(t *testing.T, h *host, query string) {
	t.Helper()
	f, ok := h.Filter()
	if !ok {
		t.Fatal("Filter reported nothing to filter")
	}
	form := filterform.New(f.Spec, filterform.WithQuery(query))
	run(t, h, h.ApplyFilter(filterform.AppliedMsg{ID: form.ID(), Values: form.Values(), Sort: form.Sort(), Query: form.Query()}))
}

func lastList(svc *fakeService) issuesvc.ListQuery {
	calls := svc.listCalls()
	return calls[len(calls)-1]
}

// shown returns the descriptions of the keys that are enabled.
func shown(layers []keyhelp.Layer) []string { return uitest.Enabled(layers) }

func TestFilterQueryFollowsTheTab(t *testing.T) {
	if _, ok := newSection(t, newFakeService(nil), 80, 20).Filter(); ok {
		t.Error("Filter offered a filter before a repository")
	}
	h := started(t, newFakeService(sampleIssues(12)), 80, 20)
	if f, ok := h.Filter(); !ok || f.Query != "is:open" || f.Subject != "eggzec/gh-tui" {
		t.Errorf("Filter = %q of %q, %v; want the open tab of the repository", f.Query, f.Subject, ok)
	}
	press(t, h, "]", "]")
	if f, _ := h.Filter(); f.Query != "" {
		t.Errorf("Filter on the All tab = %q, want no state", f.Query)
	}
}

func TestApplyFilter(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	apply(t, h, "label:bug author:octocat sort:updated-desc")
	want := issuesvc.ListQuery{Repo: testRepo, State: core.FilterAll, Filter: "label:bug author:octocat"}
	if got := lastList(svc); got != want {
		t.Errorf("listed %+v, want %+v: All without a state, and no default sort", got, want)
	}
	if h.tab != core.FilterAll || h.Chips() != "bug · @octocat" {
		t.Errorf("tab %q, chips %q; want all with the filters as chips", h.tab, h.Chips())
	}
	if it, ok := h.list.Item(0); !ok || it.Number != 1000 || h.list.Len() != 1 {
		t.Errorf("rows = %d, first #%d; want #1000 alone, the bug by octocat", h.list.Len(), it.Number)
	}
	apply(t, h, "is:closed assignee:hubot sort:comments-asc")
	want = issuesvc.ListQuery{Repo: testRepo, State: core.FilterClosed, Filter: "assignee:hubot sort:comments-asc"}
	if got := lastList(svc); got != want {
		t.Errorf("listed %+v, want %+v", got, want)
	}
	if f, _ := h.Filter(); f.Query != "is:closed assignee:hubot sort:comments-asc" {
		t.Errorf("Filter = %q, want the form to open on what was applied", f.Query)
	}
}

func TestTabsKeepTheFilterUntilCleared(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	if slices.Contains(shown(h.KeyLayers()), "clear filters") {
		t.Error("help offers to clear filters before any")
	}
	apply(t, h, "is:open label:bug")
	press(t, h, "]")
	if want := (issuesvc.ListQuery{Repo: testRepo, State: core.FilterClosed, Filter: "label:bug"}); lastList(svc) != want {
		t.Errorf("listed %+v after ], want %+v", lastList(svc), want)
	}
	if !slices.Contains(shown(h.KeyLayers()), "clear filters") {
		t.Errorf("help %v, want clear filters while filtered", shown(h.KeyLayers()))
	}
	press(t, h, "F")
	if want := (issuesvc.ListQuery{Repo: testRepo, State: core.FilterClosed}); lastList(svc) != want {
		t.Errorf("listed %+v after F, want %+v: the tab kept, the filters gone", lastList(svc), want)
	}
	if h.Chips() != "" {
		t.Errorf("chips = %q after F, want none", h.Chips())
	}
}

func TestFilteredEmptyText(t *testing.T) {
	h := started(t, newFakeService(sampleIssues(12)), 80, 6)
	apply(t, h, "is:open author:nobody")
	if got := ansi.Strip(h.View()); !strings.Contains(got, "No open issues match the filters. Press F to clear them.") {
		t.Errorf("empty filtered list shows\n%s", got)
	}
}

func TestFilteredListsReadNoOtherTabsAhead(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	n := len(svc.requested())
	apply(t, h, "is:open label:bug")
	want := []issuesvc.ListQuery{{Repo: testRepo, State: core.FilterOpen, Filter: "label:bug"}}
	if got := svc.requested()[n:]; !slices.Equal(got, want) {
		t.Errorf("requested %v after filtering, want only the list shown: %v", got, want)
	}
	// Switching tabs reads the others ahead only once the filter is
	// cleared.
	press(t, h, "]", "F")
	want = append(want, firstPages(core.FilterClosed, core.FilterAll)...)
	want = slices.Insert(want, 1, issuesvc.ListQuery{Repo: testRepo, State: core.FilterClosed, Filter: "label:bug"})
	if got := svc.requested()[n:]; !slices.Equal(got, want) {
		t.Errorf("requested %v, want the other tabs read ahead once cleared: %v", got, want)
	}
}

// fakeFacets offers labels, people and two milestones, which Milestones
// reads ahead.
type fakeFacets struct {
	mu    sync.Mutex
	reads []core.RepoRef
}

func (*fakeFacets) Labels(context.Context, core.RepoRef) ([]core.Label, error) {
	return []core.Label{{Name: "bug"}, {Name: "ui"}}, nil
}

func (*fakeFacets) People(context.Context, facets.PeopleQuery) ([]core.User, error) {
	return []core.User{{Login: "octocat"}}, nil
}

func (f *fakeFacets) CachedMilestones(core.RepoRef) ([]core.Milestone, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reads) == 0 {
		return nil, false
	}
	return []core.Milestone{{Number: 2, Title: "v2.0.0"}, {Number: 3, Title: "v2 polish"}}, true
}

func (f *fakeFacets) Milestones(_ context.Context, repo core.RepoRef) ([]core.Milestone, error) {
	f.mu.Lock()
	f.reads = append(f.reads, repo)
	f.mu.Unlock()
	ms, _ := f.CachedMilestones(repo)
	return ms, nil
}

func TestFilterOffersMilestones(t *testing.T) {
	fc := &fakeFacets{}
	h := started(t, newFakeService(sampleIssues(12)), 80, 20, WithFacets(fc))
	press(t, h, "down", "up")
	if len(fc.reads) != 1 {
		t.Fatalf("read milestones %d times, want once, after the list loaded", len(fc.reads))
	}
	f, _ := h.Filter()
	i := slices.IndexFunc(f.Spec.Fields, func(fl filterform.Field) bool { return fl.Key == "milestone" })
	labels := make([]string, 0, len(f.Spec.Fields[i].Options))
	for _, it := range f.Spec.Fields[i].Options {
		labels = append(labels, it.Label)
	}
	if want := []string{"Any", "None", "v2.0.0", "v2 polish"}; !slices.Equal(labels, want) {
		t.Errorf("milestones = %q, want %q", labels, want)
	}
	// A milestone round-trips through the query, quoted when it has a
	// space, and no milestone is its own qualifier.
	for _, q := range []string{`is:open milestone:"v2 polish"`, "is:open no:milestone", "is:open milestone:v9"} {
		form := filterform.New(f.Spec, filterform.WithQuery(q))
		if got := form.Query(); got != q+" sort:updated-desc" {
			t.Errorf("query %q reads back as %q", q, got)
		}
	}
	// Another repository reads its own.
	run(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "o", Name: "r"}}))
	if fc.reads[len(fc.reads)-1] == testRepo && len(fc.reads) > 1 {
		t.Errorf("reads = %v, want the other repository's", fc.reads)
	}
}

func TestClaimed(t *testing.T) {
	if newSection(t, newFakeService(nil), 80, 20).Claimed() != nil {
		t.Error("claimed keys without a repository")
	}
	h := started(t, newFakeService(sampleIssues(12)), 80, 20)
	for k, want := range map[string]bool{"]": true, "[": true, "x": false, "f": false} {
		if got := key.Matches(keyMsg(k), h.Claimed()...); got != want {
			t.Errorf("claimed %q = %v, want %v", k, got, want)
		}
	}
	if key.Matches(tea.KeyPressMsg{Code: tea.KeyLeft}, h.Claimed()...) {
		t.Error("claimed left, which isn't a key of prev_filter")
	}
}
