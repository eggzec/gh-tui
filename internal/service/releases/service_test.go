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
	"github.com/eggzec/gh-tui/internal/config"
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

	refs       []string
	heads      []string
	compareErr error
	commitErr  error
	body       string // the notes of v3 when set
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
	rel := v3
	if f.body != "" {
		rel.Body = f.body
	}
	return rel, github.Response{ETag: `"r1"`}, nil
}

func (f *fakeAPI) ListReleases(context.Context, core.RepoRef, string, int, github.Conditional) (core.Page[core.Release], github.Response, error) {
	return core.Page[core.Release]{}, github.Response{}, core.ErrNotFound
}

func (f *fakeAPI) LatestRelease(context.Context, core.RepoRef) (core.Release, bool, error) {
	return core.Release{}, false, nil
}

func (f *fakeAPI) ReleaseAssetURL(context.Context, core.RepoRef, int64) (string, error) {
	return "", core.ErrNotFound
}

// CommitOf serves a verified commit with the ETag "c1", and a 304 to it. The
// refs and heads asked for are kept.
func (f *fakeAPI) CommitOf(_ context.Context, _ core.RepoRef, ref string, cond github.Conditional) (core.Commit, github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refs = append(f.refs, ref)
	if f.commitErr != nil {
		return core.Commit{}, github.Response{}, f.commitErr
	}
	if cond.ETag == `"c1"` {
		return core.Commit{}, github.Response{NotModified: true}, nil
	}
	return core.Commit{SHA: "abc123", Verification: core.Verification{Verified: true}}, github.Response{ETag: `"c1"`}, nil
}

// Compare serves a branch 5 commits ahead with the ETag "p1", and a 304 to
// it, or the error of compareErr.
func (f *fakeAPI) Compare(_ context.Context, _ core.RepoRef, _, head string, cond github.Conditional) (core.Compare, github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heads = append(f.heads, head)
	switch {
	case f.compareErr != nil:
		return core.Compare{}, github.Response{}, f.compareErr
	case cond.ETag == `"p1"`:
		return core.Compare{}, github.Response{NotModified: true}, nil
	}
	return core.Compare{AheadBy: 5}, github.Response{ETag: `"p1"`}, nil
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
		time.Sleep(config.Default().Cache.TTL.Releases)
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
		// A release has no flags to mark; it is served as it is.
		for _, err := range []error{&github.Error{StatusCode: http.StatusBadGateway}, &core.RateLimitError{Reset: time.Now().Add(time.Hour)}} {
			api.err = err
			if r, err := s.Get(t.Context(), repo, v3.ID); err != nil || r.Tag != "v3.0.0" {
				t.Errorf("Get while GitHub fails = %+v, %v; want the cached release", r, err)
			}
		}
		api.err = nil
		if r, err := s.Get(t.Context(), repo, v3.ID); err != nil || r.Tag != "v3.0.0" {
			t.Errorf("Get after the outage = %+v, %v; want the release", r, err)
		}
		if conds := api.calls(); conds[len(conds)-1].ETag != `"r1"` {
			t.Errorf("the read after the outage asked with %+v, want the release's ETag", conds[len(conds)-1])
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
