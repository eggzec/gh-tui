package issues

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// pastTTL is long enough for a cached entry to go stale.
const pastTTL = 2 * time.Minute

func TestListFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{t: t, listIssues: func(core.StateFilter, string, github.Conditional) (core.Page[core.Issue], github.Response, error) {
		return page("", 1, 2), ok(`"v1"`), nil
	}}
	s := New(api)
	q := ListQuery{Repo: repo}
	if _, cached := s.CachedList(q); cached {
		t.Fatal("CachedList reported a page before any fetch")
	}

	for range 2 {
		got, err := s.List(t.Context(), q)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if !slices.Equal(numbers(got), []int{1, 2}) {
			t.Errorf("List = %v, want [1 2]", numbers(got))
		}
	}
	api.checkCalls(t, "ListIssues")
	if got, cached := s.CachedList(q); !cached || !slices.Equal(numbers(got), []int{1, 2}) {
		t.Errorf("CachedList = %v, %v; want [1 2], true", numbers(got), cached)
	}
}

func TestListRevalidatesStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var conds []github.Conditional
		type response struct {
			page core.Page[core.Issue]
			res  github.Response
		}
		responses := []response{
			{page("", 1), ok(`"v1"`)},
			{res: notModified},
			{page("", 3, 1), ok(`"v2"`)},
			{res: notModified},
		}
		api := &fakeAPI{t: t, listIssues: func(_ core.StateFilter, _ string, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
			conds = append(conds, cond)
			r := responses[0]
			responses = responses[1:]
			return r.page, r.res, nil
		}}
		s := New(api)
		q := ListQuery{Repo: repo}
		list := func(want ...int) {
			t.Helper()
			got, err := s.List(t.Context(), q)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if !slices.Equal(numbers(got), want) {
				t.Errorf("List = %v, want %v", numbers(got), want)
			}
		}

		list(1)
		time.Sleep(pastTTL)
		if got, cached := s.CachedList(q); !cached || !slices.Equal(numbers(got), []int{1}) {
			t.Errorf("CachedList of a stale page = %v, %v; want [1], true", numbers(got), cached)
		}
		list(1) // 304 keeps the page and makes it fresh again.
		list(1) // Fresh, so no call.
		time.Sleep(pastTTL)
		list(3, 1) // Changed, so the new page and ETag are stored.
		time.Sleep(pastTTL)
		list(3, 1)

		want := []github.Conditional{{}, {ETag: `"v1"`}, {ETag: `"v1"`}, {ETag: `"v2"`}}
		if !slices.Equal(conds, want) {
			t.Errorf("validators sent = %q, want %q", conds, want)
		}
	})
}

func TestListKeys(t *testing.T) {
	type call struct {
		state  core.StateFilter
		cursor string
	}
	var calls []call
	api := &fakeAPI{t: t, listIssues: func(state core.StateFilter, cursor string, _ github.Conditional) (core.Page[core.Issue], github.Response, error) {
		calls = append(calls, call{state, cursor})
		if cursor == "" {
			return page("https://api.github.com/repositories/1/issues?page=2", 1), ok(`"p1"`), nil
		}
		return page("", 2), ok(`"p2"`), nil
	}}
	s := New(api)

	first, err := s.List(t.Context(), ListQuery{Repo: repo})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if first.Next != "https://api.github.com/repositories/1/issues?page=2" {
		t.Fatalf("Next = %q, want the API's cursor", first.Next)
	}
	queries := []ListQuery{
		{Repo: repo, State: core.FilterOpen}, // Same entry as the zero State.
		{Repo: repo, Cursor: first.Next},
		{Repo: repo, State: core.FilterOpen, Cursor: first.Next},
		{Repo: repo, State: core.FilterClosed},
	}
	for _, q := range queries {
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List(%+v): %v", q, err)
		}
	}

	want := []call{{core.FilterOpen, ""}, {core.FilterOpen, first.Next}, {core.FilterClosed, ""}}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
	second, _ := s.CachedList(ListQuery{Repo: repo, Cursor: first.Next})
	if !slices.Equal(numbers(second), []int{2}) || !second.Last() {
		t.Errorf("second page = %+v, want [2] as the last page", second)
	}
}

func TestListError(t *testing.T) {
	notFound := fmt.Errorf("GET repos/octo-org/hello/issues: %w", core.ErrNotFound)
	api := &fakeAPI{t: t, listIssues: func(core.StateFilter, string, github.Conditional) (core.Page[core.Issue], github.Response, error) {
		return core.Page[core.Issue]{}, github.Response{}, notFound
	}}
	s := New(api)
	for range 2 {
		_, err := s.List(t.Context(), ListQuery{Repo: repo})
		if !errors.Is(err, core.ErrNotFound) || !strings.Contains(err.Error(), "list issues of octo-org/hello") {
			t.Errorf("error = %v, want a wrapped ErrNotFound", err)
		}
	}
	// Errors aren't cached, so the second List tried again.
	api.checkCalls(t, "ListIssues", "ListIssues")
	if _, cached := s.CachedList(ListQuery{Repo: repo}); cached {
		t.Error("CachedList reported a page after a failed fetch")
	}
}

func detailAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		t: t,
		getIssue: func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
			return issue(number), ok(`"i1"`), nil
		},
		listComments: func(int, github.Conditional) (core.Page[core.Comment], github.Response, error) {
			return thread(), ok(`"c1"`), nil
		},
	}
}

func TestGetFreshHitMakesNoCall(t *testing.T) {
	api := detailAPI(t)
	s := New(api)
	if _, cached := s.CachedIssue(repo, 7); cached {
		t.Fatal("CachedIssue reported an issue before any fetch")
	}
	for range 2 {
		got, err := s.Get(t.Context(), repo, 7)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Number != 7 || len(got.Thread) != 1 || got.Thread[0].ID != "IC_1" {
			t.Errorf("Get = %+v, want issue 7 with its comment", got)
		}
	}
	api.checkCalls(t, "GetIssue", "ListIssueComments")
	if got, cached := s.CachedIssue(repo, 7); !cached || got.Number != 7 || len(got.Thread) != 1 {
		t.Errorf("CachedIssue = %+v, %v; want issue 7 with its comment", got, cached)
	}
}

func TestGetRevalidatesStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := detailAPI(t)
		s := New(api)
		if _, err := s.Get(t.Context(), repo, 7); err != nil {
			t.Fatalf("Get: %v", err)
		}
		api.called()

		time.Sleep(pastTTL)
		var issueCond, commentsCond github.Conditional
		api.getIssue = func(_ int, cond github.Conditional) (core.Issue, github.Response, error) {
			issueCond = cond
			return core.Issue{}, notModified, nil
		}
		api.listComments = func(_ int, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
			commentsCond = cond
			return core.Page[core.Comment]{}, notModified, nil
		}
		got, err := s.Get(t.Context(), repo, 7)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Number != 7 || len(got.Thread) != 1 {
			t.Errorf("Get after a 304 = %+v, want the cached detail", got)
		}
		if issueCond.ETag != `"i1"` || commentsCond.ETag != `"c1"` {
			t.Errorf("validators = %q and %q, want each entry's own ETag", issueCond.ETag, commentsCond.ETag)
		}
		if _, err := s.Get(t.Context(), repo, 7); err != nil {
			t.Fatalf("Get: %v", err)
		}
		api.checkCalls(t, "GetIssue", "ListIssueComments")
	})
}

func TestGetError(t *testing.T) {
	api := detailAPI(t)
	api.listComments = func(int, github.Conditional) (core.Page[core.Comment], github.Response, error) {
		return core.Page[core.Comment]{}, github.Response{}, &core.RateLimitError{Reset: epoch}
	}
	s := New(api)
	_, err := s.Get(t.Context(), repo, 7)
	if !errors.Is(err, core.ErrRateLimited) || !strings.Contains(err.Error(), "get issue octo-org/hello#7") {
		t.Errorf("error = %v, want a wrapped ErrRateLimited", err)
	}
	if _, cached := s.CachedIssue(repo, 7); cached {
		t.Error("CachedIssue reported a detail whose comments failed to load")
	}
}
