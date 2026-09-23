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
	list func(ctx context.Context, repo core.RepoRef, state core.State, cursor string) (core.Page[core.PullRequest], error)
	get  func(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)

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

func (f *fakeAPI) ListPullRequests(ctx context.Context, repo core.RepoRef, state core.State, cursor string) (core.Page[core.PullRequest], error) {
	f.called("list")
	return f.list(ctx, repo, state, cursor)
}

func (f *fakeAPI) GetPullRequest(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error) {
	f.called("get")
	return f.get(ctx, repo, number)
}

func openPull(number int) core.PullRequest {
	return core.PullRequest{ID: fmt.Sprintf("PR_%d", number), Repo: repo, Number: number, State: core.StateOpen}
}

// listing returns pages of open pull requests: the first page holds 1 and 2
// and continues at cursor "c1", which holds 3.
func listing(_ context.Context, _ core.RepoRef, _ core.State, cursor string) (core.Page[core.PullRequest], error) {
	if cursor == "c1" {
		return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(3)}}, nil
	}
	return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(1), openPull(2)}, Next: "c1"}, nil
}

func detail(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
	return core.PullRequestDetail{
		PullRequest: openPull(number),
		Reviews:     []core.Review{{ID: "PRR_1", State: core.ReviewStateApproved}},
	}, nil
}

func TestListFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{list: listing}
	s := New(api)
	q := ListQuery{Repo: repo, State: core.StateOpen}

	if _, ok := s.Cached(q); ok {
		t.Error("Cached reported a page before any fetch")
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
	if got, ok := s.Cached(q); !ok || !reflect.DeepEqual(got, first) {
		t.Errorf("Cached = %+v, %v; want the fetched page", got, ok)
	}
}

func TestListStaleRefetches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		title := "before"
		api := &fakeAPI{list: func(context.Context, core.RepoRef, core.State, string) (core.Page[core.PullRequest], error) {
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
		if got, ok := s.Cached(q); !ok || got.Items[0].Title != "before" {
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
	}
	var (
		mu    sync.Mutex
		calls []call
	)
	api := &fakeAPI{list: func(ctx context.Context, r core.RepoRef, state core.State, cursor string) (core.Page[core.PullRequest], error) {
		mu.Lock()
		calls = append(calls, call{state, cursor})
		mu.Unlock()
		return listing(ctx, r, state, cursor)
	}}
	s := New(api)

	first, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if first.Next != "c1" {
		t.Fatalf("next = %q, want c1", first.Next)
	}
	second, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen, After: first.Next})
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

	want := []call{{core.StateOpen, ""}, {core.StateOpen, "c1"}, {core.StateClosed, ""}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
}

func TestListErrorWrappedAndNotCached(t *testing.T) {
	api := &fakeAPI{list: func(context.Context, core.RepoRef, core.State, string) (core.Page[core.PullRequest], error) {
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
	if _, ok := s.Cached(q); ok {
		t.Error("Cached reported a page after a failed fetch")
	}
}

func TestGetFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{get: detail}
	s := New(api)

	if _, ok := s.CachedDetail(repo, 1); ok {
		t.Error("CachedDetail reported a detail before any fetch")
	}
	for range 2 {
		d, err := s.Get(t.Context(), repo, 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if d.Number != 1 || len(d.Reviews) != 1 {
			t.Errorf("detail = %+v, want #1 with one review", d)
		}
	}
	if n := api.count("get"); n != 1 {
		t.Errorf("API called %d times, want 1", n)
	}
	if d, ok := s.CachedDetail(repo, 1); !ok || d.Number != 1 {
		t.Errorf("CachedDetail = %+v, %v; want #1", d, ok)
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
		if _, ok := s.CachedDetail(repo, 1); !ok {
			t.Error("CachedDetail lost the stale detail")
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
	if _, ok := s.CachedDetail(repo, 1); ok {
		t.Error("#1 is still cached beyond the capacity of 1")
	}
}
