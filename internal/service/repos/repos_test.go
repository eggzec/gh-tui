package repos

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

// fakeAPI answers with its func fields and counts the calls. A nil field
// fails the test when it is called.
type fakeAPI struct {
	t *testing.T

	listRepos func(first int, after string) (core.Page[core.Repo], error)
	getRepo   func(ref core.RepoRef) (core.Repo, error)
	star      func(ref core.RepoRef, starred bool) error

	mu    sync.Mutex
	calls []string
}

func (f *fakeAPI) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

// Calls returns the calls made so far, such as "list 30 c1" or "get o/r".
func (f *fakeAPI) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeAPI) ListRepos(_ context.Context, first int, after string) (core.Page[core.Repo], error) {
	f.record(fmt.Sprintf("list %d %s", first, after))
	if f.listRepos == nil {
		f.t.Error("unexpected ListRepos")
		return core.Page[core.Repo]{}, errors.New("unexpected call")
	}
	return f.listRepos(first, after)
}

func (f *fakeAPI) GetRepo(_ context.Context, ref core.RepoRef) (core.Repo, error) {
	f.record("get " + ref.String())
	if f.getRepo == nil {
		f.t.Error("unexpected GetRepo")
		return core.Repo{}, errors.New("unexpected call")
	}
	return f.getRepo(ref)
}

func (f *fakeAPI) Star(_ context.Context, ref core.RepoRef) error {
	return f.setStarred(ref, true)
}

func (f *fakeAPI) Unstar(_ context.Context, ref core.RepoRef) error {
	return f.setStarred(ref, false)
}

func (f *fakeAPI) setStarred(ref core.RepoRef, starred bool) error {
	verb := "star"
	if !starred {
		verb = "unstar"
	}
	f.record(verb + " " + ref.String())
	if f.star == nil {
		f.t.Error("unexpected " + verb)
		return errors.New("unexpected call")
	}
	return f.star(ref, starred)
}

func (f *fakeAPI) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	if got := f.Calls(); !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

var (
	ghTUI    = core.Repo{ID: "R_1", Ref: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, Stars: 42, Language: "Go"}
	dotfiles = core.Repo{ID: "R_2", Ref: core.RepoRef{Owner: "octo-org", Name: "dotfiles"}, Stars: 7, Starred: true}
)

func page(next string, repos ...core.Repo) core.Page[core.Repo] {
	return core.Page[core.Repo]{Items: repos, Next: next}
}

func equalPage(a, b core.Page[core.Repo]) bool {
	return a.Next == b.Next && slices.Equal(a.Items, b.Items)
}

func TestListFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{t: t, listRepos: func(int, string) (core.Page[core.Repo], error) {
		return page("c1", ghTUI, dotfiles), nil
	}}
	s := New(api)

	if _, ok := s.CachedList(ListQuery{}); ok {
		t.Error("CachedList hit before any List")
	}
	for range 2 {
		got, err := s.List(t.Context(), ListQuery{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if want := page("c1", ghTUI, dotfiles); !equalPage(got, want) {
			t.Errorf("List = %+v, want %+v", got, want)
		}
	}
	api.wantCalls(t, "list 30 ")

	if got, ok := s.CachedList(ListQuery{First: DefaultPageSize}); !ok || len(got.Items) != 2 {
		t.Errorf("CachedList = %+v, %v; want the page, since First 0 means the default", got, ok)
	}
}

func TestListPagination(t *testing.T) {
	api := &fakeAPI{t: t, listRepos: func(_ int, after string) (core.Page[core.Repo], error) {
		if after == "" {
			return page("c1", ghTUI), nil
		}
		return page("", dotfiles), nil
	}}
	s := New(api)

	first, err := s.List(t.Context(), ListQuery{First: 1})
	if err != nil || first.Next != "c1" {
		t.Fatalf("first page = %+v, %v; want Next c1", first, err)
	}
	second, err := s.List(t.Context(), ListQuery{First: 1, After: first.Next})
	if err != nil || !second.Last() || !slices.Equal(second.Items, []core.Repo{dotfiles}) {
		t.Fatalf("second page = %+v, %v; want the last page with dotfiles", second, err)
	}
	api.wantCalls(t, "list 1 ", "list 1 c1")

	// Each cursor and size is its own entry.
	if got, ok := s.CachedList(ListQuery{First: 1}); !ok || got.Next != "c1" {
		t.Errorf("cached first page = %+v, %v", got, ok)
	}
	if _, ok := s.CachedList(ListQuery{First: 2}); ok {
		t.Error("a different page size hit the cache")
	}
}

func TestListStaleRevalidates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stars := 42
		api := &fakeAPI{t: t, listRepos: func(int, string) (core.Page[core.Repo], error) {
			r := ghTUI
			r.Stars = stars
			return page("", r), nil
		}}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.List(t.Context(), ListQuery{}); err != nil {
			t.Fatal(err)
		}

		stars = 43
		time.Sleep(time.Minute)
		if got, ok := s.CachedList(ListQuery{}); !ok || got.Items[0].Stars != 42 {
			t.Errorf("stale CachedList = %+v, %v; want the old page", got, ok)
		}
		got, err := s.List(t.Context(), ListQuery{})
		if err != nil || got.Items[0].Stars != 43 {
			t.Errorf("List = %+v, %v; want the refetched page", got, err)
		}
		api.wantCalls(t, "list 30 ", "list 30 ")
		if got, _ := s.CachedList(ListQuery{}); got.Items[0].Stars != 43 {
			t.Errorf("CachedList = %+v, want the refetched page stored", got)
		}
	})
}

func TestGetFreshAndStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		desc := "v1"
		api := &fakeAPI{t: t, getRepo: func(core.RepoRef) (core.Repo, error) {
			r := ghTUI
			r.Description = desc
			return r, nil
		}}
		s := New(api, WithTTL(time.Minute))

		if _, ok := s.CachedGet(ghTUI.Ref); ok {
			t.Error("CachedGet hit before any Get")
		}
		for range 2 {
			if got, err := s.Get(t.Context(), ghTUI.Ref); err != nil || got.Description != "v1" {
				t.Fatalf("Get = %+v, %v; want v1", got, err)
			}
		}
		api.wantCalls(t, "get eggzec/gh-tui")

		// Owners and names are case-insensitive, so this is the same entry.
		upper := core.RepoRef{Owner: "Eggzec", Name: "GH-TUI"}
		if got, ok := s.CachedGet(upper); !ok || got.Description != "v1" {
			t.Errorf("CachedGet(%v) = %+v, %v; want v1", upper, got, ok)
		}

		desc = "v2"
		time.Sleep(time.Minute)
		if got, ok := s.CachedGet(ghTUI.Ref); !ok || got.Description != "v1" {
			t.Errorf("stale CachedGet = %+v, %v; want v1", got, ok)
		}
		if got, err := s.Get(t.Context(), ghTUI.Ref); err != nil || got.Description != "v2" {
			t.Errorf("Get = %+v, %v; want v2", got, err)
		}
		api.wantCalls(t, "get eggzec/gh-tui", "get eggzec/gh-tui")
	})
}

func TestReadErrors(t *testing.T) {
	notFound := fmt.Errorf("graphql: %w", core.ErrNotFound)
	api := &fakeAPI{
		t:         t,
		listRepos: func(int, string) (core.Page[core.Repo], error) { return core.Page[core.Repo]{}, &core.RateLimitError{} },
		getRepo:   func(core.RepoRef) (core.Repo, error) { return core.Repo{}, notFound },
	}
	s := New(api)

	_, err := s.List(t.Context(), ListQuery{})
	if !errors.Is(err, core.ErrRateLimited) || err.Error() == (&core.RateLimitError{}).Error() {
		t.Errorf("List error = %v, want a wrapped rate limit", err)
	}
	if _, ok := s.CachedList(ListQuery{}); ok {
		t.Error("a failed List was cached")
	}

	_, err = s.Get(t.Context(), ghTUI.Ref)
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Get error = %v, want ErrNotFound", err)
	}
	if want := "get repo eggzec/gh-tui: " + notFound.Error(); err.Error() != want {
		t.Errorf("Get error = %q, want %q", err, want)
	}
	if _, ok := s.CachedGet(ghTUI.Ref); ok {
		t.Error("a failed Get was cached")
	}
}
