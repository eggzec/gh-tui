package pulls

import (
	"context"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	"github.com/eggzec/gh-tui/internal/service/pulls"
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
	drain(t, h, h.ApplyFilter(filterform.AppliedMsg{ID: form.ID(), Values: form.Values(), Sort: form.Sort(), Query: form.Query()}))
}

// shown returns the descriptions of the keys that are enabled.
func shown(layers []keyhelp.Layer) []string { return uitest.Enabled(layers) }

// firstLists returns the first pages listed, in order.
func firstLists(svc *fakeService) []pulls.ListQuery {
	var qs []pulls.ListQuery
	for _, q := range svc.listed() {
		if q.Cursor == "" {
			qs = append(qs, q)
		}
	}
	return qs
}

func TestFilterQueryFollowsTheTab(t *testing.T) {
	h := newTest(t, newFakeService(), 80, 20)
	if _, ok := h.Filter(); ok {
		t.Error("Filter offered a filter before a repository")
	}
	h = started(t, newFakeService(), 80, 20)
	f, ok := h.Filter()
	if !ok || f.Query != "is:open" || f.Subject != "eggzec/gh-tui" {
		t.Errorf("Filter = %q of %q, %v; want the open tab of the repository", f.Query, f.Subject, ok)
	}
	press(t, h, "]")
	press(t, h, "]")
	press(t, h, "]")
	if f, _ := h.Filter(); f.Query != "" {
		t.Errorf("Filter on the All tab = %q, want no state", f.Query)
	}
}

func TestApplyFilter(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	apply(t, h, "is:merged author:hubot -is:draft sort:updated-desc")
	want := pulls.ListQuery{Repo: repo, State: core.StateMerged, Filter: "author:hubot -is:draft"}
	if got := firstLists(svc); got[len(got)-1] != want {
		t.Errorf("listed %+v, want %+v without the state and the default sort", got[len(got)-1], want)
	}
	if h.tab != core.StateMerged || h.Chips() != "@hubot · -is:draft" {
		t.Errorf("tab %q, chips %q; want merged with the filters as chips", h.tab, h.Chips())
	}
	if got := screen(h); !strings.Contains(got, "Rename the watch package") || strings.Contains(got, "Cache ETags") {
		t.Errorf("filtered list shows\n%s", got)
	}
	if f, _ := h.Filter(); f.Query != "is:merged author:hubot -is:draft" {
		t.Errorf("Filter = %q, want the form to open on what was applied", f.Query)
	}

	// Applying it again lists nothing more.
	n := len(svc.listed())
	apply(t, h, "is:merged author:hubot -is:draft")
	if len(svc.listed()) != n {
		t.Error("applying the filters in force listed again")
	}
}

func TestFilterSortAndReview(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	apply(t, h, "is:open review:approved label:cache sort:created-asc")
	got := firstLists(svc)
	if want := "review:approved label:cache sort:created-asc"; got[len(got)-1].Filter != want {
		t.Errorf("filter = %q, want %q", got[len(got)-1].Filter, want)
	}
	if h.Chips() != "review:approved · cache · sort:created-asc" {
		t.Errorf("chips = %q", h.Chips())
	}
}

func TestTabsKeepTheFilter(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	apply(t, h, "is:open author:octocat")
	press(t, h, "]")
	got := firstLists(svc)
	if want := (pulls.ListQuery{Repo: repo, State: core.StateClosed, Filter: "author:octocat"}); got[len(got)-1] != want {
		t.Errorf("listed %+v after ], want %+v: the next tab with the filter", got[len(got)-1], want)
	}
	if f, _ := h.Filter(); f.Query != "is:closed author:octocat" {
		t.Errorf("Filter = %q, want the tab and the filter", f.Query)
	}
}

func TestClearFilter(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	if descs := shown(h.KeyLayers()); slices.Contains(descs, "clear filters") {
		t.Error("help offers to clear filters before any")
	}
	apply(t, h, "is:closed author:vilmibm")
	if descs := shown(h.KeyLayers()); !slices.Contains(descs, "clear filters") {
		t.Errorf("help %v, want clear filters while filtered", descs)
	}
	press(t, h, "F")
	got := firstLists(svc)
	if want := (pulls.ListQuery{Repo: repo, State: core.StateClosed}); got[len(got)-1] != want {
		t.Errorf("listed %+v after F, want %+v: the tab kept, the filters gone", got[len(got)-1], want)
	}
	if h.Chips() != "" || h.query != "" {
		t.Errorf("chips %q, query %q; want none", h.Chips(), h.query)
	}
}

func TestFilteredEmptyText(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 90, 6)
	apply(t, h, "is:open author:nobody")
	if got := screen(h); !strings.Contains(got, "No open pull requests match the filters. Press F to clear them.") {
		t.Errorf("empty filtered list shows\n%s", got)
	}
}

func TestFilteredListsReadNoOtherTabsAhead(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30, WithFilterPrefetch())
	n := len(svc.requested())
	apply(t, h, "is:open author:octocat")
	want := make([]pulls.ListQuery, 0, 2)
	want = append(want, pulls.ListQuery{Repo: repo, State: core.StateOpen, Filter: "author:octocat"})
	if got := svc.requested()[n:]; !slices.Equal(got, want) {
		t.Errorf("requested %v after filtering, want only the list shown: %v", got, want)
	}
	// Switching tabs reads the others ahead only once the filter is
	// cleared.
	press(t, h, "]")
	press(t, h, "F")
	want = append(want,
		pulls.ListQuery{Repo: repo, State: core.StateClosed, Filter: "author:octocat"},
		pulls.ListQuery{Repo: repo, State: core.StateClosed},
		pulls.ListQuery{Repo: repo, State: core.StateMerged})
	if got := svc.requested()[n:]; !slices.Equal(got, want) {
		t.Errorf("requested %v, want the other tabs read ahead once cleared: %v", got, want)
	}
}

func TestFilterBeforeTheFirstPageReadsAheadOnceCleared(t *testing.T) {
	svc := newFakeService()
	h := newTest(t, svc, 80, 30, WithFilterPrefetch())
	drain(t, h, h.Update(ui.RepoMsg{Repo: repo}))
	// The list isn't loaded, so nothing is read ahead yet.
	h.started = true
	apply(t, h, "is:open label:cache")
	press(t, h, "F")
	press(t, h, "]")
	want := []pulls.ListQuery{
		{Repo: repo, State: core.StateOpen, Filter: "label:cache"},
		{Repo: repo, State: core.StateOpen},
		{Repo: repo, State: core.StateClosed},
		{Repo: repo, State: core.StateMerged},
	}
	if got := svc.requested(); !slices.Equal(got, want) {
		t.Errorf("requested %v, want the other tabs read once the filter is cleared and the tab switched: %v", got, want)
	}
}

func TestClaimed(t *testing.T) {
	h := newTest(t, newFakeService(), 80, 20)
	if h.Claimed() != nil {
		t.Error("claimed keys without a repository")
	}
	h = started(t, newFakeService(), 80, 20)
	for k, want := range map[string]bool{"]": true, "[": true, "x": false, "f": false} {
		if got := key.Matches(keyMsg(k), h.Claimed()...); got != want {
			t.Errorf("claimed %q = %v, want %v", k, got, want)
		}
	}
	if key.Matches(tea.KeyPressMsg{Code: tea.KeyRight}, h.Claimed()...) {
		t.Error("claimed right, which isn't a key of next_filter")
	}
}

// fakeFacets offers two labels and the people whose login has the text.
type fakeFacets struct{}

func (fakeFacets) Labels(context.Context, core.RepoRef) ([]core.Label, error) {
	return []core.Label{{Name: "bug", Description: "Something isn't working"}, {Name: "cache"}}, nil
}

func (fakeFacets) People(_ context.Context, q facets.PeopleQuery) ([]core.User, error) {
	all := []core.User{{Login: "octocat", Name: "The Octocat"}, {Login: "hubot"}}
	return slices.DeleteFunc(all, func(u core.User) bool { return !strings.Contains(u.Login, q.Text) }), nil
}

func TestFilterLoadsFacets(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
	f, _ := h.Filter()
	for _, fl := range f.Spec.Fields {
		if fl.Load != nil {
			t.Errorf("field %s loads without facets", fl.Key)
		}
	}
	h = started(t, newFakeService(), 80, 20, WithFacets(fakeFacets{}))
	f, _ = h.Filter()
	loads := map[string]filterform.Loader{}
	for _, fl := range f.Spec.Fields {
		if fl.Load != nil {
			loads[fl.Key] = fl.Load
		}
	}
	labels, err := loads["labels"](t.Context(), "")
	if err != nil || len(labels) != 2 || labels[0] != (filterform.Item{Label: "bug", Value: "bug", Detail: "Something isn't working"}) {
		t.Errorf("labels = %+v, %v", labels, err)
	}
	people, err := loads["author"](t.Context(), "oct")
	if err != nil || len(people) != 1 || people[0].Value != "octocat" || people[0].Detail != "The Octocat" {
		t.Errorf("people = %+v, %v", people, err)
	}
	if loads["assignee"] == nil {
		t.Error("the assignee loads no people")
	}
}
