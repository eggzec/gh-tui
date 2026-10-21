package search

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

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
