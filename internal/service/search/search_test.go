package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

// fakeAPI answers with its func fields and records the calls. A nil field
// fails the test when it is called.
type fakeAPI struct {
	t *testing.T

	repos  func(query, cursor string, perPage int) (core.Page[core.Repo], error)
	issues func(query, cursor string, perPage int) (core.Page[core.SearchHit], error)

	mu    sync.Mutex
	calls []string
}

func (f *fakeAPI) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeAPI) SearchRepos(_ context.Context, query, cursor string, perPage int) (core.Page[core.Repo], error) {
	f.record(fmt.Sprintf("repos %q %d %s", query, perPage, cursor))
	if f.repos == nil {
		f.t.Error("unexpected SearchRepos")
		return core.Page[core.Repo]{}, errors.New("unexpected call")
	}
	return f.repos(query, cursor, perPage)
}

func (f *fakeAPI) SearchIssues(_ context.Context, query, cursor string, perPage int) (core.Page[core.SearchHit], error) {
	f.record(fmt.Sprintf("issues %q %d %s", query, perPage, cursor))
	if f.issues == nil {
		f.t.Error("unexpected SearchIssues")
		return core.Page[core.SearchHit]{}, errors.New("unexpected call")
	}
	return f.issues(query, cursor, perPage)
}

// wantCalls compares the calls in any order, since a search of every kind
// makes its two calls at once.
func (f *fakeAPI) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	f.mu.Lock()
	got := slices.Clone(f.calls)
	f.mu.Unlock()
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

var (
	ghTUI  = core.Repo{ID: "R_1", Ref: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}}
	crash  = core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{ID: "I_1", Repo: ghTUI.Ref, Number: 42, Title: "Crash"}}
	fixPR  = core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{ID: "PR_1", Repo: ghTUI.Ref, Number: 7, Title: "Fix crash"}}
	repoIt = core.SearchHit{Kind: core.SearchRepos, Repo: ghTUI}
)

// repoPage returns the last page of a repository search.
func repoPage(repos ...core.Repo) core.Page[core.Repo] {
	return core.Page[core.Repo]{Items: repos}
}

func hitPage(next string, hits ...core.SearchHit) core.Page[core.SearchHit] {
	return core.Page[core.SearchHit]{Items: hits, Next: next}
}

func ids(p core.Page[core.SearchHit]) []string {
	out := make([]string, len(p.Items))
	for i := range p.Items {
		h := &p.Items[i]
		if h.Kind == core.SearchRepos {
			out[i] = h.Repo.ID
		} else {
			out[i] = h.Issue.ID
		}
	}
	return out
}

func TestSearchKinds(t *testing.T) {
	tests := []struct {
		name      string
		q         Query
		wantCalls []string
		wantIDs   []string
	}{
		{
			name:      "repos",
			q:         Query{Text: "tui", Kind: core.SearchRepos},
			wantCalls: []string{`repos "tui" 20 `},
			wantIDs:   []string{"R_1"},
		},
		{
			name:      "issues",
			q:         Query{Text: "crash", Kind: core.SearchIssues, PageSize: 5},
			wantCalls: []string{`issues "crash is:issue" 5 `},
			wantIDs:   []string{"I_1", "PR_1"},
		},
		{
			name:      "pulls",
			q:         Query{Text: "  fix   crash ", Kind: core.SearchPulls, PageSize: 500},
			wantCalls: []string{`issues "fix crash is:pr" 100 `},
			wantIDs:   []string{"I_1", "PR_1"},
		},
		{
			name:      "all lists repositories first",
			q:         Query{Text: "crash"},
			wantCalls: []string{`repos "crash" 20 `, `issues "crash" 20 `},
			wantIDs:   []string{"R_1", "I_1", "PR_1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{
				t:      t,
				repos:  func(string, string, int) (core.Page[core.Repo], error) { return repoPage(ghTUI), nil },
				issues: func(string, string, int) (core.Page[core.SearchHit], error) { return hitPage("", crash, fixPR), nil },
			}
			s := New(api)
			got, err := s.Search(t.Context(), tt.q)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if !slices.Equal(ids(got), tt.wantIDs) || !got.Last() {
				t.Errorf("Search = %v next %q, want %v on the last page", ids(got), got.Next, tt.wantIDs)
			}
			api.wantCalls(t, tt.wantCalls...)
			if h := got.Items[0]; h.Kind == core.SearchRepos && h.Repo != repoIt.Repo {
				t.Errorf("repo hit = %+v, want %+v", got.Items[0], repoIt)
			}
		})
	}
}

func TestSearchCaches(t *testing.T) {
	api := &fakeAPI{t: t, repos: func(string, string, int) (core.Page[core.Repo], error) {
		return repoPage(ghTUI), nil
	}}
	s := New(api)
	q := Query{Text: "TUI", Kind: core.SearchRepos}
	if _, ok := s.CachedSearch(q); ok {
		t.Error("CachedSearch hit before any Search")
	}
	for _, text := range []string{"TUI", "tui", " tui "} {
		if _, err := s.Search(t.Context(), Query{Text: text, Kind: core.SearchRepos}); err != nil {
			t.Fatal(err)
		}
	}
	api.wantCalls(t, `repos "TUI" 20 `)
	if got, ok := s.CachedSearch(Query{Text: "tui", Kind: core.SearchRepos, PageSize: DefaultPageSize}); !ok || len(got.Items) != 1 {
		t.Errorf("CachedSearch = %+v, %v; want the page", got, ok)
	}
	// Each kind is its own entry.
	if _, ok := s.CachedSearch(Query{Text: "tui"}); ok {
		t.Error("CachedSearch of every kind hit the repository search")
	}
}

func TestSearchEmptyText(t *testing.T) {
	api := &fakeAPI{t: t}
	s := New(api)
	got, err := s.Search(t.Context(), Query{Text: "   "})
	if err != nil || len(got.Items) != 0 || !got.Last() {
		t.Errorf("Search = %+v, %v; want an empty last page", got, err)
	}
	if got, ok := s.CachedSearch(Query{}); !ok || len(got.Items) != 0 {
		t.Errorf("CachedSearch = %+v, %v; want a known empty page", got, ok)
	}
	api.wantCalls(t)
}

func TestSearchAllPages(t *testing.T) {
	api := &fakeAPI{
		t: t,
		repos: func(_, cursor string, _ int) (core.Page[core.Repo], error) {
			if cursor != "" {
				t.Errorf("repos asked for page %q after the last", cursor)
			}
			return repoPage(ghTUI), nil
		},
		issues: func(_, cursor string, _ int) (core.Page[core.SearchHit], error) {
			if cursor == "" {
				return hitPage("https://api/search/issues?page=2&q=a%26b", crash), nil
			}
			return hitPage("", fixPR), nil
		},
	}
	s := New(api)
	first, err := s.Search(t.Context(), Query{Text: "a&b"})
	if err != nil || first.Last() || !slices.Equal(ids(first), []string{"R_1", "I_1"}) {
		t.Fatalf("first page = %v next %q, %v; want R_1 I_1 and more", ids(first), first.Next, err)
	}
	second, err := s.Search(t.Context(), Query{Text: "a&b", Cursor: first.Next})
	if err != nil || !second.Last() || !slices.Equal(ids(second), []string{"PR_1"}) {
		t.Fatalf("second page = %v next %q, %v; want PR_1 on the last page", ids(second), second.Next, err)
	}
	api.wantCalls(t, `repos "a&b" 20 `, `issues "a&b" 20 `, `issues "a&b" 20 https://api/search/issues?page=2&q=a%26b`)
}

func TestSearchBadCursor(t *testing.T) {
	s := New(&fakeAPI{t: t})
	if _, err := s.Search(t.Context(), Query{Text: "x", Cursor: "%zz"}); err == nil {
		t.Error("Search with a malformed cursor succeeded")
	}
}

func TestSearchErrors(t *testing.T) {
	limited := fmt.Errorf("search repos: %w", &core.RateLimitError{Reset: time.Unix(1790000000, 0)})
	for _, kind := range []core.SearchKind{core.SearchRepos, core.SearchAll} {
		t.Run(string(kind), func(t *testing.T) {
			api := &fakeAPI{
				t:      t,
				repos:  func(string, string, int) (core.Page[core.Repo], error) { return core.Page[core.Repo]{}, limited },
				issues: func(string, string, int) (core.Page[core.SearchHit], error) { return hitPage("", crash), nil },
			}
			s := New(api)
			_, err := s.Search(t.Context(), Query{Text: "tui", Kind: kind})
			var rl *core.RateLimitError
			if !errors.Is(err, core.ErrRateLimited) || !errors.As(err, &rl) {
				t.Fatalf("error = %v, want a rate limit", err)
			}
			if _, ok := s.CachedSearch(Query{Text: "tui", Kind: kind}); ok {
				t.Error("a failed search was cached")
			}
		})
	}
	t.Run("issues fail", func(t *testing.T) {
		boom := errors.New("boom")
		api := &fakeAPI{
			t:      t,
			repos:  func(string, string, int) (core.Page[core.Repo], error) { return repoPage(ghTUI), nil },
			issues: func(string, string, int) (core.Page[core.SearchHit], error) { return core.Page[core.SearchHit]{}, boom },
		}
		if _, err := New(api).Search(t.Context(), Query{Text: "tui"}); !errors.Is(err, boom) {
			t.Errorf("error = %v, want boom", err)
		}
	})
}

func TestSearchExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := 0
		api := &fakeAPI{t: t, repos: func(string, string, int) (core.Page[core.Repo], error) {
			n++
			r := ghTUI
			r.Stars = n
			return repoPage(r), nil
		}}
		s := New(api)
		q := Query{Text: "tui", Kind: core.SearchRepos}
		if _, err := s.Search(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		time.Sleep(DefaultTTL)
		if got, ok := s.CachedSearch(q); !ok || got.Items[0].Repo.Stars != 1 {
			t.Errorf("stale CachedSearch = %+v, %v; want the old page", got, ok)
		}
		got, err := s.Search(t.Context(), q)
		if err != nil || got.Items[0].Repo.Stars != 2 {
			t.Errorf("Search = %+v, %v; want the refetched page", got, err)
		}

		s.Invalidate()
		if got, err = s.Search(t.Context(), q); err != nil || got.Items[0].Repo.Stars != 3 {
			t.Errorf("Search after Invalidate = %+v, %v; want a new fetch", got, err)
		}
	})
}

func TestOptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t, repos: func(string, string, int) (core.Page[core.Repo], error) {
			return repoPage(ghTUI), nil
		}}
		s := New(api, WithTTL(time.Hour), WithCapacity(1))
		for _, text := range []string{"a", "b", "a"} {
			if _, err := s.Search(t.Context(), Query{Text: text, Kind: core.SearchRepos}); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(time.Minute)
		if _, err := s.Search(t.Context(), Query{Text: "a", Kind: core.SearchRepos}); err != nil {
			t.Fatal(err)
		}
		// A capacity of one evicts a when b comes in; an hour's TTL keeps
		// the last a fresh.
		api.wantCalls(t, `repos "a" 20 `, `repos "b" 20 `, `repos "a" 20 `)
	})
}
