package search

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func (f *fakeAPI) SearchCommits(_ context.Context, query, cursor string, perPage int) (core.SearchPage[core.SearchCommit], error) {
	f.record(fmt.Sprintf("commits %q %d %s", query, perPage, cursor))
	if f.commits == nil {
		f.t.Error("unexpected SearchCommits")
		return core.SearchPage[core.SearchCommit]{}, errors.New("unexpected call")
	}
	return f.commits(query, cursor, perPage)
}

// A refused commit search tells when the limit lifts, isn't cached, and runs
// once the limit lifts.
func TestCommitsRateLimited(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(42 * time.Second)
		limited := true
		api := &fakeAPI{t: t, commits: func(string, string, int) (core.SearchPage[core.SearchCommit], error) {
			if limited {
				return core.SearchPage[core.SearchCommit]{}, fmt.Errorf("search commits: %w", &core.RateLimitError{Reset: reset})
			}
			return core.SearchPage[core.SearchCommit]{Items: []core.SearchCommit{{SHA: "abc"}}, Total: 1}, nil
		}}
		s := New(api)
		_, err := s.Commits(t.Context(), CommitsQuery{Text: "a"})
		rl, ok := errors.AsType[*core.RateLimitError](err)
		if !ok || !errors.Is(err, core.ErrRateLimited) || !rl.Reset.Equal(reset) {
			t.Fatalf("error = %v, want a rate limit until %v", err, reset)
		}
		if _, ok := s.CachedCommits(CommitsQuery{Text: "a"}); ok {
			t.Error("a refused search was cached")
		}

		limited = false
		time.Sleep(42 * time.Second)
		if got, err := s.Commits(t.Context(), CommitsQuery{Text: "a"}); err != nil || len(got.Items) != 1 {
			t.Errorf("Commits after the reset = %+v, %v", got, err)
		}
		api.wantCalls(t, `commits "a" 20 `, `commits "a" 20 `)
	})
}

func TestCommitsCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t, commits: func(string, string, int) (core.SearchPage[core.SearchCommit], error) {
			return core.SearchPage[core.SearchCommit]{
				Items: []core.SearchCommit{{SHA: "abc"}},
				Total: 1,
			}, nil
		}}
		s := New(api)
		q := CommitsQuery{Text: "  fix repo:a/b "}
		if _, ok := s.CachedCommits(q); ok {
			t.Error("CachedCommits hit before any search")
		}
		got, err := s.Commits(t.Context(), q)
		if err != nil || got.Total != 1 || len(got.Items) != 1 {
			t.Fatalf("Commits = %+v, %v", got, err)
		}
		if _, err := s.Commits(t.Context(), CommitsQuery{Text: "FIX repo:a/b"}); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.CachedCommits(q); !ok {
			t.Error("CachedCommits missed after a search")
		}
		api.wantCalls(t, `commits "fix repo:a/b" 20 `)
	})
}
