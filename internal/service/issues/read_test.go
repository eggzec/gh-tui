package issues

import (
	"errors"
	"fmt"
	"reflect"
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
	api := &fakeAPI{t: t, listIssues: func(core.StateFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
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
		api := &fakeAPI{t: t, listIssues: func(_ core.StateFilter, _ string, _ int, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
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
		state   core.StateFilter
		cursor  string
		perPage int
	}
	var calls []call
	api := &fakeAPI{t: t, listIssues: func(state core.StateFilter, cursor string, perPage int, _ github.Conditional) (core.Page[core.Issue], github.Response, error) {
		calls = append(calls, call{state, cursor, perPage})
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
		// Same entry as the zero State and PageSize.
		{Repo: repo, State: core.FilterOpen, PageSize: 30},
		{Repo: repo, Cursor: first.Next},
		{Repo: repo, State: core.FilterOpen, Cursor: first.Next},
		{Repo: repo, State: core.FilterClosed},
		{Repo: repo, PageSize: 10},
		{Repo: repo, PageSize: 10, Cursor: first.Next},
		// Clamped to the most GitHub returns, so the second is a hit.
		{Repo: repo, PageSize: 500},
		{Repo: repo, PageSize: maxPageSize},
	}
	for _, q := range queries {
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List(%+v): %v", q, err)
		}
	}

	want := []call{
		{core.FilterOpen, "", 30},
		{core.FilterOpen, first.Next, 30},
		{core.FilterClosed, "", 30},
		{core.FilterOpen, "", 10},
		{core.FilterOpen, first.Next, 10},
		{core.FilterOpen, "", maxPageSize},
	}
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
	api := &fakeAPI{t: t, listIssues: func(core.StateFilter, string, int, github.Conditional) (core.Page[core.Issue], github.Response, error) {
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
	// No listComments, so a detail read that asks for comments fails.
	return &fakeAPI{
		t: t,
		getIssue: func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
			return issue(number), ok(`"i1"`), nil
		},
	}
}

func TestGetFreshHitMakesNoCall(t *testing.T) {
	api := detailAPI(t)
	s := New(api)
	if _, cached := s.CachedGet(repo, 7); cached {
		t.Fatal("CachedGet reported an issue before any fetch")
	}
	for range 2 {
		got, err := s.Get(t.Context(), repo, 7)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Number != 7 {
			t.Errorf("Get = %+v, want issue 7", got)
		}
	}
	api.checkCalls(t, "GetIssue")
	if got, cached := s.CachedGet(repo, 7); !cached || got.Number != 7 {
		t.Errorf("CachedGet = %+v, %v; want issue 7", got, cached)
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
		var cond github.Conditional
		api.getIssue = func(_ int, c github.Conditional) (core.Issue, github.Response, error) {
			cond = c
			return core.Issue{}, notModified, nil
		}
		got, err := s.Get(t.Context(), repo, 7)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Number != 7 {
			t.Errorf("Get after a 304 = %+v, want the cached issue", got)
		}
		if cond.ETag != `"i1"` {
			t.Errorf("validator = %q, want the stored ETag", cond.ETag)
		}
		if _, err := s.Get(t.Context(), repo, 7); err != nil {
			t.Fatalf("Get: %v", err)
		}
		api.checkCalls(t, "GetIssue")
	})
}

func TestGetError(t *testing.T) {
	api := detailAPI(t)
	api.getIssue = func(int, github.Conditional) (core.Issue, github.Response, error) {
		return core.Issue{}, github.Response{}, &core.RateLimitError{Reset: epoch}
	}
	s := New(api)
	_, err := s.Get(t.Context(), repo, 7)
	if !errors.Is(err, core.ErrRateLimited) || !strings.Contains(err.Error(), "get issue octo-org/hello#7") {
		t.Errorf("error = %v, want a wrapped ErrRateLimited", err)
	}
	if _, cached := s.CachedGet(repo, 7); cached {
		t.Error("CachedGet reported an issue that failed to load")
	}
}

func TestCommentsKeys(t *testing.T) {
	type call struct {
		number  int
		cursor  string
		perPage int
	}
	var calls []call
	api := &fakeAPI{t: t, listComments: func(number int, cursor string, perPage int, _ github.Conditional) (core.Page[core.Comment], github.Response, error) {
		calls = append(calls, call{number, cursor, perPage})
		return commentPage(t, thread(3), cursor, perPage), ok(`"c1"`), nil
	}}
	s := New(api)
	q := CommentsQuery{Repo: repo, Number: 7, PageSize: 2}
	if _, cached := s.CachedComments(q); cached {
		t.Fatal("CachedComments reported a page before any fetch")
	}

	first, err := s.Comments(t.Context(), q)
	if err != nil {
		t.Fatalf("Comments: %v", err)
	}
	if !slices.Equal(ids(first), []string{"IC_1", "IC_2"}) || first.Next != "offset=2" {
		t.Fatalf("first page = %+v, want IC_1 and IC_2 with the API's cursor", first)
	}
	queries := []CommentsQuery{
		{Repo: repo, Number: 7, PageSize: 2, Cursor: first.Next},
		{Repo: repo, Number: 7, PageSize: 2}, // Cached above.
		{Repo: repo, Number: 7},
		{Repo: repo, Number: 7, PageSize: 30}, // Same as the zero PageSize.
		{Repo: repo, Number: 7, PageSize: 1, Cursor: first.Next},
		{Repo: repo, Number: 7, PageSize: 2, Cursor: first.Next}, // Cached above.
		{Repo: repo, Number: 8, PageSize: 2},
		{Repo: repo, Number: 7, PageSize: 500},
	}
	for _, q := range queries {
		if _, err := s.Comments(t.Context(), q); err != nil {
			t.Fatalf("Comments(%+v): %v", q, err)
		}
	}

	want := []call{
		{7, "", 2},
		{7, first.Next, 2},
		{7, "", 30},
		{7, first.Next, 1},
		{8, "", 2},
		{7, "", maxPageSize},
	}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
	last, cached := s.CachedComments(CommentsQuery{Repo: repo, Number: 7, PageSize: 2, Cursor: first.Next})
	if !cached || !slices.Equal(ids(last), []string{"IC_3"}) || !last.Last() {
		t.Errorf("second page = %+v, %v; want IC_3 as the last page", last, cached)
	}
}

func TestCommentsRevalidatesStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Each page has its own ETag, and a later version of the thread
		// changes the ETags of the pages that changed.
		version := 1
		conds := map[string][]github.Conditional{}
		api := &fakeAPI{t: t, listComments: func(_ int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
			conds[cursor] = append(conds[cursor], cond)
			etag := fmt.Sprintf(`"%s@%d"`, cursor, version)
			if cursor == "" {
				etag = `"first"` // The first page never changes.
			}
			if cond.ETag == etag {
				return core.Page[core.Comment]{}, notModified, nil
			}
			return commentPage(t, thread(2+version), cursor, perPage), ok(etag), nil
		}}
		s := New(api)
		first := CommentsQuery{Repo: repo, Number: 7, PageSize: 2}
		second := CommentsQuery{Repo: repo, Number: 7, PageSize: 2, Cursor: "offset=2"}
		get := func(q CommentsQuery, want ...string) {
			t.Helper()
			got, err := s.Comments(t.Context(), q)
			if err != nil {
				t.Fatalf("Comments: %v", err)
			}
			if !slices.Equal(ids(got), want) {
				t.Errorf("Comments(%q) = %q, want %q", q.Cursor, ids(got), want)
			}
		}

		get(first, "IC_1", "IC_2")
		get(second, "IC_3")
		time.Sleep(pastTTL)
		if got, cached := s.CachedComments(second); !cached || !slices.Equal(ids(got), []string{"IC_3"}) {
			t.Errorf("CachedComments of a stale page = %q, %v; want IC_3, true", ids(got), cached)
		}
		get(first, "IC_1", "IC_2") // 304 keeps each page and makes it fresh again.
		get(second, "IC_3")
		get(first, "IC_1", "IC_2") // Fresh, so no call.
		get(second, "IC_3")
		time.Sleep(pastTTL)
		version = 2
		get(first, "IC_1", "IC_2")
		get(second, "IC_3", "IC_4") // Changed, so the new page is stored.

		want := map[string][]github.Conditional{
			"":         {{}, {ETag: `"first"`}, {ETag: `"first"`}},
			"offset=2": {{}, {ETag: `"offset=2@1"`}, {ETag: `"offset=2@1"`}},
		}
		if !reflect.DeepEqual(conds, want) {
			t.Errorf("validators sent = %q, want %q", conds, want)
		}
	})
}

func TestCommentsError(t *testing.T) {
	api := &fakeAPI{t: t, listComments: func(int, string, int, github.Conditional) (core.Page[core.Comment], github.Response, error) {
		return core.Page[core.Comment]{}, github.Response{}, &core.RateLimitError{Reset: epoch}
	}}
	s := New(api)
	q := CommentsQuery{Repo: repo, Number: 7}
	_, err := s.Comments(t.Context(), q)
	if !errors.Is(err, core.ErrRateLimited) || !strings.Contains(err.Error(), "list comments of issue octo-org/hello#7") {
		t.Errorf("error = %v, want a wrapped ErrRateLimited", err)
	}
	if _, cached := s.CachedComments(q); cached {
		t.Error("CachedComments reported a page that failed to load")
	}
}

func TestInvalidateRevalidatesRepo(t *testing.T) {
	// The reads run one at a time, so the fakes need no lock.
	var conds []github.Conditional
	cond := func(c github.Conditional) { conds = append(conds, c) }
	api := &fakeAPI{
		t: t,
		listIssues: func(_ core.StateFilter, _ string, _ int, c github.Conditional) (core.Page[core.Issue], github.Response, error) {
			cond(c)
			if c.ETag != "" {
				return core.Page[core.Issue]{}, notModified, nil
			}
			return page("", 7), ok(`"l1"`), nil
		},
		getIssue: func(number int, c github.Conditional) (core.Issue, github.Response, error) {
			cond(c)
			if c.ETag != "" {
				return core.Issue{}, notModified, nil
			}
			return issue(number), ok(`"i1"`), nil
		},
		listComments: func(_ int, cursor string, perPage int, c github.Conditional) (core.Page[core.Comment], github.Response, error) {
			cond(c)
			if c.ETag != "" {
				return core.Page[core.Comment]{}, notModified, nil
			}
			return commentPage(t, thread(2), cursor, perPage), ok(`"c1"`), nil
		},
	}
	s := New(api)
	lq, cq := ListQuery{Repo: repo}, CommentsQuery{Repo: repo, Number: 7}
	// The list goes last: a list page that GitHub confirmed vouches for the
	// issue, which would spare the reads after it the request.
	read := func() {
		t.Helper()
		if it, err := s.Get(t.Context(), repo, 7); err != nil || it.Number != 7 {
			t.Fatalf("Get = %+v, %v; want issue 7", it, err)
		}
		if p, err := s.Comments(t.Context(), cq); err != nil || len(p.Items) != 2 {
			t.Fatalf("Comments = %v, %v; want 2 comments", ids(p), err)
		}
		if p, err := s.List(t.Context(), lq); err != nil || !slices.Equal(numbers(p), []int{7}) {
			t.Fatalf("List = %v, %v; want [7]", numbers(p), err)
		}
	}
	read()
	api.checkCalls(t, "ListIssues", "GetIssue", "ListIssueComments")

	s.Invalidate(repo)
	if _, cached := s.CachedList(lq); !cached {
		t.Error("list page was dropped, want it kept stale")
	}
	if _, cached := s.CachedGet(repo, 7); !cached {
		t.Error("issue was dropped, want it kept stale")
	}
	if _, cached := s.CachedComments(cq); !cached {
		t.Error("comments were dropped, want them kept stale")
	}
	read() // Each read asks GitHub, which answers 304.
	api.checkCalls(t, "ListIssues", "GetIssue", "ListIssueComments")
	read() // The 304s made the entries fresh again.
	api.checkCalls(t)

	slices.SortFunc(conds, func(a, b github.Conditional) int { return strings.Compare(a.ETag, b.ETag) })
	want := []github.Conditional{{}, {}, {}, {ETag: `"c1"`}, {ETag: `"i1"`}, {ETag: `"l1"`}}
	if !slices.Equal(conds, want) {
		t.Errorf("validators sent = %q, want %q", conds, want)
	}
}
