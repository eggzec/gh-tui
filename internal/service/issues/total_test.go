package issues

import (
	"errors"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func totalAPI(t *testing.T, n int) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		t: t,
		listIssues: func(core.StateFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
			return core.Page[core.Issue]{Items: []core.Issue{issue(1)}, Next: "n"}, github.Response{}, nil
		},
		filterIssues: func(github.IssueFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
			return core.Page[core.Issue]{Items: []core.Issue{issue(1)}}, github.Response{}, nil
		},
		search: func(string, string, int) (core.Page[core.SearchHit], error) {
			return core.Page[core.SearchHit]{Items: []core.SearchHit{{Kind: core.SearchIssues, Issue: issue(2)}}}, nil
		},
		count: func(string) (int, error) { return n, nil },
	}
}

// The first page carries the total of a search of the list's own text, for
// the list, a list with a filter and a search alike.
func TestListTotalAsksTheListsSearch(t *testing.T) {
	for _, tt := range []struct {
		name  string
		q     ListQuery
		count string
	}{
		{"open", ListQuery{Repo: repo, State: core.FilterOpen}, "repo:octo-org/hello is:issue is:open sort:updated-desc"},
		{"all", ListQuery{Repo: repo, State: core.FilterAll}, "repo:octo-org/hello is:issue sort:updated-desc"},
		{"labels", ListQuery{Repo: repo, State: core.FilterClosed, Filter: "label:bug"}, "repo:octo-org/hello is:issue is:closed label:bug sort:updated-desc"},
		{"assignee", ListQuery{Repo: repo, Filter: "assignee:ann"}, "repo:octo-org/hello is:issue is:open assignee:ann sort:updated-desc"},
		{"author", ListQuery{Repo: repo, Filter: "author:bob"}, "repo:octo-org/hello is:issue is:open author:bob sort:updated-desc"},
		{"milestone", ListQuery{Repo: repo, Filter: "milestone:v1"}, "repo:octo-org/hello is:issue is:open milestone:v1 sort:updated-desc"},
		{"text", ListQuery{Repo: repo, Filter: "crash on start"}, "repo:octo-org/hello is:issue is:open crash on start sort:updated-desc"},
		{"negation", ListQuery{Repo: repo, Filter: "-label:wip sort:created-asc"}, "repo:octo-org/hello is:issue is:open -label:wip sort:created-asc"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := totalAPI(t, 42)
			s := New(api)
			p, err := s.List(t.Context(), tt.q)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if p.Total != 42 {
				t.Errorf("Total = %d, want 42", p.Total)
			}
			if want := []string{tt.count}; !slices.Equal(api.counted, want) {
				t.Errorf("counted %q, want %q", api.counted, want)
			}
			if got := searchQuery(tt.q.normalize(0)); got != tt.count {
				t.Errorf("list's search = %q, want %q", got, tt.count)
			}
			if c, ok := s.CachedList(tt.q); !ok || c.Total != 42 {
				t.Errorf("CachedList = %d, %v; want the total kept", c.Total, ok)
			}
		})
	}
}

// A failed probe leaves the total unknown and the page whole.
func TestListTotalFailsSilent(t *testing.T) {
	api := totalAPI(t, 0)
	api.count = func(string) (int, error) { return 0, errors.New("boom") }
	p, err := New(api).List(t.Context(), ListQuery{Repo: repo})
	if err != nil || len(p.Items) != 1 || p.Total != 0 {
		t.Errorf("List = %d items, total %d, %v; want 1 item, no total, no error", len(p.Items), p.Total, err)
	}
}

// Later pages ask nothing: the first page holds the total.
func TestListTotalOnlyOnFirstPage(t *testing.T) {
	api := totalAPI(t, 42)
	p, err := New(api).List(t.Context(), ListQuery{Repo: repo, Cursor: "n"})
	if err != nil || p.Total != 0 || len(api.counted) != 0 {
		t.Errorf("List = total %d, %v, counted %q; want none", p.Total, err, api.counted)
	}
}

// A failed list has no total to show.
func TestListTotalNotOnError(t *testing.T) {
	api := totalAPI(t, 42)
	api.listIssues = func(core.StateFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
		return core.Page[core.Issue]{}, github.Response{}, errors.New("down")
	}
	if _, err := New(api).List(t.Context(), ListQuery{Repo: repo}); err == nil {
		t.Error("List succeeded, want the error")
	}
}
