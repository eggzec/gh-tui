package pulls

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

var repo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

// fakeAPI is an API whose methods are set per test. It counts the calls of
// each method.
type fakeAPI struct {
	list func(ctx context.Context, repo core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error)
	get  func(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	// comments and reviews back the timeline reads.
	comments func(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Comment], error)
	reviews  func(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Review], error)
	id       func(ctx context.Context, repo core.RepoRef, number int) (string, error)
	// mutate backs every mutation. Method names the mutation; merge also
	// passes the merge method.
	mutate func(ctx context.Context, method, id string, how core.MergeMethod) (core.PullRequest, error)

	mu    sync.Mutex
	calls map[string]int
}

func (f *fakeAPI) called(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls[method]++
}

// count returns how often method was called.
func (f *fakeAPI) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *fakeAPI) ListPullRequests(ctx context.Context, repo core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
	f.called("list")
	return f.list(ctx, repo, state, cursor, first)
}

func (f *fakeAPI) GetPullRequest(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error) {
	f.called("get")
	return f.get(ctx, repo, number)
}

func (f *fakeAPI) ListPullRequestComments(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Comment], error) {
	f.called("comments")
	return f.comments(ctx, repo, number, cursor, first)
}

func (f *fakeAPI) ListPullRequestReviews(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Review], error) {
	f.called("reviews")
	return f.reviews(ctx, repo, number, cursor, first)
}

func (f *fakeAPI) PullRequestID(ctx context.Context, repo core.RepoRef, number int) (string, error) {
	f.called("id")
	return f.id(ctx, repo, number)
}

func (f *fakeAPI) MergePullRequest(ctx context.Context, id string, method core.MergeMethod) (core.PullRequest, error) {
	f.called("merge")
	return f.mutate(ctx, "merge", id, method)
}

func (f *fakeAPI) ClosePullRequest(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("close")
	return f.mutate(ctx, "close", id, "")
}

func (f *fakeAPI) ReopenPullRequest(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("reopen")
	return f.mutate(ctx, "reopen", id, "")
}

func (f *fakeAPI) MarkPullRequestReady(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("ready")
	return f.mutate(ctx, "ready", id, "")
}

func (f *fakeAPI) ConvertPullRequestToDraft(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("draft")
	return f.mutate(ctx, "draft", id, "")
}

func openPull(number int) core.PullRequest {
	return core.PullRequest{ID: fmt.Sprintf("PR_%d", number), Repo: repo, Number: number, State: core.StateOpen}
}

// listing returns pages of open pull requests: the first page holds 1 and 2
// and continues at cursor "c1", which holds 3.
func listing(_ context.Context, _ core.RepoRef, _ core.State, cursor string, _ int) (core.Page[core.PullRequest], error) {
	if cursor == "c1" {
		return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(3)}}, nil
	}
	return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(1), openPull(2)}, Next: "c1"}, nil
}

func detail(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
	return core.PullRequestDetail{
		PullRequest: openPull(number),
		CheckRuns:   []core.CheckRun{{Name: "test", Status: "completed", Conclusion: "success"}},
	}, nil
}

func TestListFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{list: listing}
	s := New(api)
	q := ListQuery{Repo: repo, State: core.StateOpen}

	if _, ok := s.CachedList(q); ok {
		t.Error("CachedList reported a page before any fetch")
	}
	first, err := s.List(t.Context(), q)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	second, err := s.List(t.Context(), q)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := api.count("list"); n != 1 {
		t.Errorf("API called %d times, want 1", n)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cached page = %+v, want %+v", second, first)
	}
	if got, ok := s.CachedList(q); !ok || !reflect.DeepEqual(got, first) {
		t.Errorf("Cached = %+v, %v; want the fetched page", got, ok)
	}
}

func TestListStaleRefetches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		title := "before"
		api := &fakeAPI{list: func(context.Context, core.RepoRef, core.State, string, int) (core.Page[core.PullRequest], error) {
			pr := openPull(1)
			pr.Title = title
			return core.Page[core.PullRequest]{Items: []core.PullRequest{pr}}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		q := ListQuery{Repo: repo, State: core.StateOpen}
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List: %v", err)
		}

		time.Sleep(2 * time.Minute)
		title = "after"
		if got, ok := s.CachedList(q); !ok || got.Items[0].Title != "before" {
			t.Errorf("Cached = %+v, %v; want the stale page", got, ok)
		}
		got, err := s.List(t.Context(), q)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if n := api.count("list"); n != 2 {
			t.Errorf("API called %d times, want 2", n)
		}
		if got.Items[0].Title != "after" {
			t.Errorf("title = %q, want the refetched one", got.Items[0].Title)
		}
	})
}

func TestListKeysByQuery(t *testing.T) {
	type call struct {
		state  core.State
		cursor string
		first  int
	}
	var (
		mu    sync.Mutex
		calls []call
	)
	api := &fakeAPI{list: func(ctx context.Context, r core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
		mu.Lock()
		calls = append(calls, call{state, cursor, first})
		mu.Unlock()
		return listing(ctx, r, state, cursor, first)
	}}
	s := New(api)

	first, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if first.Next != "c1" {
		t.Fatalf("next = %q, want c1", first.Next)
	}
	second, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen, Cursor: first.Next})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Number != 3 || !second.Last() {
		t.Errorf("second page = %+v, want the last page with #3", second)
	}
	if _, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateClosed}); err != nil {
		t.Fatalf("List: %v", err)
	}
	// The same query with other casing is the same repository.
	upper := core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}
	if _, err := s.List(t.Context(), ListQuery{Repo: upper, State: core.StateOpen}); err != nil {
		t.Fatalf("List: %v", err)
	}
	// Zero and negative sizes mean the default, and sizes above GitHub's
	// maximum are clamped to it, so each pair shares a page.
	for _, size := range []int{defaultPageSize, -1, 10, 10, 500, maxPageSize} {
		if _, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen, PageSize: size}); err != nil {
			t.Fatalf("List: %v", err)
		}
	}

	want := []call{
		{core.StateOpen, "", defaultPageSize},
		{core.StateOpen, "c1", defaultPageSize},
		{core.StateClosed, "", defaultPageSize},
		{core.StateOpen, "", 10},
		{core.StateOpen, "", maxPageSize},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
}

func TestListErrorWrappedAndNotCached(t *testing.T) {
	api := &fakeAPI{list: func(context.Context, core.RepoRef, core.State, string, int) (core.Page[core.PullRequest], error) {
		return core.Page[core.PullRequest]{}, fmt.Errorf("graphql: %w", core.ErrNotFound)
	}}
	s := New(api)
	q := ListQuery{Repo: repo}

	for range 2 {
		_, err := s.List(t.Context(), q)
		if !errors.Is(err, core.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
		if err != nil && !strings.Contains(err.Error(), "list pulls of eggzec/gh-tui") {
			t.Errorf("error %q lacks context", err)
		}
	}
	if n := api.count("list"); n != 2 {
		t.Errorf("API called %d times, want 2: errors must not be cached", n)
	}
	if _, ok := s.CachedList(q); ok {
		t.Error("CachedList reported a page after a failed fetch")
	}
}

func TestGetFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{get: detail}
	s := New(api)

	if _, ok := s.CachedGet(repo, 1); ok {
		t.Error("CachedGet reported a detail before any fetch")
	}
	for range 2 {
		d, err := s.Get(t.Context(), repo, 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if d.Number != 1 || len(d.CheckRuns) != 1 {
			t.Errorf("detail = %+v, want #1 with one check", d)
		}
	}
	if n := api.count("get"); n != 1 {
		t.Errorf("API called %d times, want 1", n)
	}
	if d, ok := s.CachedGet(repo, 1); !ok || d.Number != 1 {
		t.Errorf("CachedGet = %+v, %v; want #1", d, ok)
	}
	if _, err := s.Get(t.Context(), repo, 2); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n := api.count("get"); n != 2 {
		t.Errorf("API called %d times, want 2: another number is another entry", n)
	}
}

func TestGetStaleRefetches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{get: detail}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Get(t.Context(), repo, 1); err != nil {
			t.Fatalf("Get: %v", err)
		}
		time.Sleep(2 * time.Minute)
		if _, ok := s.CachedGet(repo, 1); !ok {
			t.Error("CachedGet lost the stale detail")
		}
		if _, err := s.Get(t.Context(), repo, 1); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if n := api.count("get"); n != 2 {
			t.Errorf("API called %d times, want 2", n)
		}
	})
}

func TestGetErrorWrapped(t *testing.T) {
	api := &fakeAPI{get: func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
		return core.PullRequestDetail{}, &core.RateLimitError{Reset: time.Now()}
	}}
	s := New(api)

	_, err := s.Get(t.Context(), repo, 7)
	if !errors.Is(err, core.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
	if _, ok := errors.AsType[*core.RateLimitError](err); !ok {
		t.Errorf("error %v does not hold the RateLimitError", err)
	}
	if err != nil && !strings.Contains(err.Error(), "get pull eggzec/gh-tui#7") {
		t.Errorf("error %q lacks context", err)
	}
}

func TestWithCapacity(t *testing.T) {
	api := &fakeAPI{get: detail}
	s := New(api, WithCapacity(1))
	for _, n := range []int{1, 2} {
		if _, err := s.Get(t.Context(), repo, n); err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	if _, ok := s.CachedGet(repo, 1); ok {
		t.Error("#1 is still cached beyond the capacity of 1")
	}
}

func TestInvalidateRefetchesRepo(t *testing.T) {
	api := &fakeAPI{list: listing, get: detail}
	api.comments = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Comment], error) {
		return core.Page[core.Comment]{Items: []core.Comment{{ID: "c"}}}, nil
	}
	api.reviews = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Review], error) {
		return core.Page[core.Review]{Items: []core.Review{{ID: "r"}}}, nil
	}
	s := New(api)
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	read := func(repo core.RepoRef) {
		t.Helper()
		ctx := t.Context()
		if _, err := s.List(ctx, ListQuery{Repo: repo, State: core.StateOpen}); err != nil {
			t.Fatalf("List: %v", err)
		}
		if _, err := s.Get(ctx, repo, 1); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if _, err := s.Comments(ctx, CommentsQuery{Repo: repo, Number: 1}); err != nil {
			t.Fatalf("Comments: %v", err)
		}
		if _, err := s.Reviews(ctx, ReviewsQuery{Repo: repo, Number: 1}); err != nil {
			t.Fatalf("Reviews: %v", err)
		}
	}
	read(repo)
	read(other)

	s.Invalidate(core.RepoRef{Owner: "EggZec", Name: "GH-TUI"})
	if _, ok := s.CachedList(ListQuery{Repo: repo, State: core.StateOpen}); !ok {
		t.Error("list page was dropped, want it kept stale")
	}
	if _, ok := s.CachedGet(repo, 1); !ok {
		t.Error("detail was dropped, want it kept stale")
	}
	read(repo)
	read(other)
	for _, m := range []string{"list", "get", "comments", "reviews"} {
		// One call per repository before, and one more for the
		// invalidated one; the other repository stays fresh.
		if n := api.count(m); n != 3 {
			t.Errorf("%s called %d times, want 3", m, n)
		}
	}
}
