package issues

import (
	"reflect"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// pathAPI returns a fake whose filtered list and search record what they
// were asked. The search finds issue 8 and a pull request, #9.
func pathAPI(t *testing.T, filters *[]github.IssueFilter, searches *[]string) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		t: t,
		filterIssues: func(f github.IssueFilter, _ string, _ int, _ github.Conditional) (core.Page[core.Issue], github.Response, error) {
			*filters = append(*filters, f)
			return page("", 7), github.Response{ETag: `"f"`}, nil
		},
		search: func(q, _ string, _ int) (core.Page[core.SearchHit], error) {
			*searches = append(*searches, q)
			return core.Page[core.SearchHit]{Items: []core.SearchHit{
				{Kind: core.SearchIssues, Issue: issue(8)},
				{Kind: core.SearchPulls, Issue: issue(9)},
			}}, nil
		},
		viewerLogin: func() (string, error) { return "octocat", nil },
	}
}

func TestListPicksThePath(t *testing.T) {
	tests := []struct {
		name   string
		state  core.StateFilter
		filter string
		list   *github.IssueFilter
		search string
	}{
		{name: "labels, people and sort", state: core.FilterAll, filter: `label:bug label:"good first issue" author:hubot mentions:@me sort:comments-asc`,
			list: &github.IssueFilter{State: core.FilterAll, Labels: []string{"bug", "good first issue"}, Creator: "hubot", Mentioned: "octocat", Sort: "comments", Asc: true}},
		{name: "assigned to me", filter: "assignee:@me",
			list: &github.IssueFilter{State: core.FilterOpen, Assignee: "octocat"}},
		{name: "nobody's, without a milestone", state: core.FilterClosed, filter: "no:assignee no:milestone",
			list: &github.IssueFilter{State: core.FilterClosed, Assignee: "none", Milestone: "none"}},
		{name: "any of two labels", filter: "label:bug,ui",
			search: "repo:octo-org/hello is:issue is:open label:bug,ui sort:updated-desc"},
		{name: "milestone", state: core.FilterAll, filter: `milestone:"v2.0.0"`,
			search: `repo:octo-org/hello is:issue milestone:"v2.0.0" sort:updated-desc`},
		{name: "negated", state: core.FilterClosed, filter: "-label:wontfix sort:created-desc",
			search: "repo:octo-org/hello is:issue is:closed -label:wontfix sort:created-desc"},
		{name: "two assignees", filter: "assignee:a assignee:b",
			search: "repo:octo-org/hello is:issue is:open assignee:a assignee:b sort:updated-desc"},
		{name: "words", filter: "crash",
			search: "repo:octo-org/hello is:issue is:open crash sort:updated-desc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				filters  []github.IssueFilter
				searches []string
			)
			s := New(pathAPI(t, &filters, &searches))
			p, err := s.List(t.Context(), ListQuery{Repo: repo, State: tt.state, Filter: tt.filter})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if tt.list != nil {
				if len(filters) != 1 || !reflect.DeepEqual(filters[0], *tt.list) || len(searches) != 0 {
					t.Errorf("filters %+v, searches %q; want the list with %+v", filters, searches, *tt.list)
				}
				return
			}
			if len(searches) != 1 || searches[0] != tt.search || len(filters) != 0 {
				t.Errorf("searches %q, filters %+v; want the search %q", searches, filters, tt.search)
			}
			if len(p.Items) != 1 || p.Items[0].Number != 8 {
				t.Errorf("page = %+v, want issue 8 without the pull request", p.Items)
			}
		})
	}
}

func TestListReadsTheViewerOnce(t *testing.T) {
	var (
		filters  []github.IssueFilter
		searches []string
	)
	api := pathAPI(t, &filters, &searches)
	s := New(api)
	for _, f := range []string{"assignee:@me", "author:@me", "mentions:@me"} {
		if _, err := s.List(t.Context(), ListQuery{Repo: repo, Filter: f}); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	api.checkCalls(t, "ViewerLogin", "FilterIssues", "FilterIssues", "FilterIssues")

	// With the viewer given, it isn't read at all.
	s = New(api, WithViewer("hubot"))
	if _, err := s.List(t.Context(), ListQuery{Repo: repo, Filter: "assignee:@me"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	api.checkCalls(t, "FilterIssues")
	if filters[len(filters)-1].Assignee != "hubot" {
		t.Errorf("assignee = %q, want the viewer given", filters[len(filters)-1].Assignee)
	}
}

func TestListSearchesWithoutTheViewer(t *testing.T) {
	var (
		filters  []github.IssueFilter
		searches []string
	)
	api := pathAPI(t, &filters, &searches)
	api.viewerLogin = func() (string, error) { return "", errUnexpected }
	s := New(api)
	if _, err := s.List(t.Context(), ListQuery{Repo: repo, Filter: "assignee:@me"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(searches) != 1 || searches[0] != "repo:octo-org/hello is:issue is:open assignee:@me sort:updated-desc" {
		t.Errorf("searches = %q, want @me left to the search", searches)
	}
}

func TestListCachesEachFilter(t *testing.T) {
	var (
		filters  []github.IssueFilter
		searches []string
	)
	api := pathAPI(t, &filters, &searches)
	api.listIssues = func(core.StateFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
		return page("", 1), github.Response{ETag: `"l"`}, nil
	}
	s := New(api, WithViewer("octocat"))
	q := ListQuery{Repo: repo, Filter: "label:bug"}
	for _, q := range []ListQuery{q, q, {Repo: repo, Filter: "label:ui"}, {Repo: repo}} {
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	api.checkCalls(t, "FilterIssues", "FilterIssues", "ListIssues")
	if p, ok := s.CachedList(q); !ok || p.Items[0].Number != 7 {
		t.Errorf("CachedList = %+v, %v; want the filtered page", p, ok)
	}
	// A stale filtered page is revalidated with its ETag.
	s.Invalidate(repo)
	api.filterIssues = func(_ github.IssueFilter, _ string, _ int, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
		if cond.ETag != `"f"` {
			t.Errorf("If-None-Match = %q, want the page's ETag", cond.ETag)
		}
		return core.Page[core.Issue]{}, github.Response{NotModified: true}, nil
	}
	if p, err := s.List(t.Context(), q); err != nil || p.Items[0].Number != 7 {
		t.Errorf("List after Invalidate = %+v, %v; want the page revalidated", p, err)
	}
}

func TestFilteredListsAreNotKept(t *testing.T) {
	store := cachetest.Aged(openStore(t), time.Hour)
	var (
		filters  []github.IssueFilter
		searches []string
	)
	api := pathAPI(t, &filters, &searches)
	api.listIssues = func(core.StateFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
		return page("", 1), github.Response{ETag: `"l"`}, nil
	}
	filtered := ListQuery{Repo: repo, Filter: "crash"}
	s := New(api, WithStore(store))
	for _, q := range []ListQuery{{Repo: repo}, filtered} {
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	api.called()

	s = New(api, WithStore(store))
	if p, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil || !p.Stale {
		t.Errorf("plain list in a new session = %+v, %v; want the kept page", p, err)
	}
	if p, err := s.List(t.Context(), filtered); err != nil || p.Stale {
		t.Errorf("filtered list in a new session = %+v, %v; want it read again", p, err)
	}
	api.checkCalls(t, "SearchIssues")
}
