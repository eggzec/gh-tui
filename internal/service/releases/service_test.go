package releases

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var repo = core.RepoRef{Owner: "charmbracelet", Name: "glow"}

var v3 = core.Release{
	ID: 368759772, Tag: "v3.0.0", Name: "v3.0.0", Body: "# Glow v3",
	Assets: []core.ReleaseAsset{{Name: "checksums.txt", Size: 5300, Downloads: 12662}},
}

// fakeAPI serves v3 with an ETag, and a 304 to a request that has it.
type fakeAPI struct {
	mu    sync.Mutex
	conds []github.Conditional
	err   error
}

func (f *fakeAPI) GetRelease(_ context.Context, r core.RepoRef, id int64, cond github.Conditional) (core.Release, github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conds = append(f.conds, cond)
	switch {
	case f.err != nil:
		return core.Release{}, github.Response{}, f.err
	case r != repo || id != v3.ID:
		return core.Release{}, github.Response{}, core.ErrNotFound
	case cond.ETag == `"r1"`:
		return core.Release{}, github.Response{NotModified: true, ETag: `"r1"`}, nil
	}
	return v3, github.Response{ETag: `"r1"`}, nil
}

func (f *fakeAPI) calls() []github.Conditional {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]github.Conditional(nil), f.conds...)
}

func get(t *testing.T, s *Service) core.Release {
	t.Helper()
	r, err := s.Get(t.Context(), repo, v3.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return r
}

func TestGetCaches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{}
		s := New(api)
		if _, ok := s.CachedGet(repo, v3.ID); ok || s.Current(repo, v3.ID) {
			t.Fatal("a release is cached before it was read")
		}
		if r := get(t, s); r.Tag != "v3.0.0" {
			t.Errorf("Get = %+v, want v3", r)
		}
		if r, ok := s.CachedGet(repo, v3.ID); !ok || r.Tag != "v3.0.0" || !s.Current(repo, v3.ID) {
			t.Errorf("CachedGet = %+v, %v; want v3, current", r, ok)
		}
		// The owner's case doesn't matter to GitHub.
		if _, err := s.Get(t.Context(), core.RepoRef{Owner: "CharmBracelet", Name: "glow"}, v3.ID); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if n := len(api.calls()); n != 1 {
			t.Errorf("requests = %d, want 1: the release is fresh", n)
		}

		// Past the TTL, the release is revalidated with its ETag.
		time.Sleep(DefaultTTL)
		if s.Current(repo, v3.ID) {
			t.Error("Current past the TTL")
		}
		if r := get(t, s); r.Tag != "v3.0.0" {
			t.Errorf("Get after a 304 = %+v, want v3", r)
		}
		if c := api.calls(); len(c) != 2 || c[1].ETag != `"r1"` {
			t.Errorf("requests = %+v, want a second one with the ETag", c)
		}
	})
}

func TestGetErrors(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	if _, err := s.Get(t.Context(), repo, 1); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Get of a missing release = %v, want core.ErrNotFound", err)
	}
	api.err = &github.Error{StatusCode: http.StatusBadGateway}
	if _, err := s.Get(t.Context(), repo, v3.ID); err == nil {
		t.Error("Get with nothing cached while GitHub is down succeeded")
	}
}

func TestGetOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{}
		s := New(api, WithTTL(time.Minute))
		get(t, s)
		time.Sleep(time.Minute)
		api.err = &github.Error{StatusCode: http.StatusBadGateway}
		if r := get(t, s); r.Tag != "v3.0.0" {
			t.Errorf("Get while GitHub is down = %+v, want the cached release", r)
		}
	})
}

func TestKeptRelease(t *testing.T) {
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	get(t, New(&fakeAPI{}, WithStore(store)))

	// A later session starts from what the first kept.
	api := &fakeAPI{}
	s := New(api, WithStore(store))
	if r := get(t, s); r.Tag != "v3.0.0" {
		t.Errorf("Get = %+v, want the kept release", r)
	}
	if n := len(api.calls()); n != 0 {
		t.Errorf("requests = %d, want none: the kept release is fresh", n)
	}
}
