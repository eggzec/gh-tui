package pulls

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
)

const behindBase = "fedcba9876543210fedcba9876543210fedcba98"

var behindQ = BehindQuery{Repo: repo, Head: commitSHA, Base: behindBase}

// behindAPI serves two commits of a base that is 250 ahead, and counts the
// requests.
func behindAPI(err error) (*fakeAPI, *atomic.Int32) {
	var n atomic.Int32
	return &fakeAPI{behind: func(_ context.Context, _ core.RepoRef, head, base string) (core.BaseCommits, error) {
		n.Add(1)
		if err != nil {
			return core.BaseCommits{}, err
		}
		if head != commitSHA || base != behindBase {
			return core.BaseCommits{}, errors.New("asked for the wrong commits")
		}
		return core.BaseCommits{AheadBy: 250, Commits: []core.Commit{{SHA: "a1"}, {SHA: "a2"}}}, nil
	}}, &n
}

func TestBehindReadsOnceAndCaches(t *testing.T) {
	api, n := behindAPI(nil)
	s := New(api)
	if _, ok := s.CachedBehind(behindQ); ok {
		t.Error("CachedBehind before a read = true, want false")
	}
	for range 2 {
		b, err := s.Behind(t.Context(), behindQ)
		if err != nil || b.AheadBy != 250 || len(b.Commits) != 2 {
			t.Fatalf("Behind = %+v, %v; want 2 of 250 commits", b, err)
		}
	}
	if n.Load() != 1 {
		t.Errorf("requests = %d, want 1: the second read is cached", n.Load())
	}
	if _, ok := s.CachedBehind(behindQ); !ok {
		t.Error("CachedBehind after a read = false, want true")
	}
}

func TestBehindErrors(t *testing.T) {
	for _, refused := range []error{core.ErrForbidden, core.ErrConflict} {
		api, n := behindAPI(refused)
		s := New(api)
		if _, err := s.Behind(t.Context(), behindQ); !errors.Is(err, refused) {
			t.Fatalf("Behind = %v, want %v", err, refused)
		}
		if _, ok := s.CachedBehind(behindQ); ok {
			t.Errorf("a failed read of %v was cached", refused)
		}
		if _, err := s.Behind(t.Context(), behindQ); err == nil || n.Load() != 2 {
			t.Errorf("after %v: err = %v, requests = %d; want the read tried again", refused, err, n.Load())
		}
	}
}

func TestBehindNeedFullSHAs(t *testing.T) {
	api, n := behindAPI(nil)
	s := New(api)
	for _, q := range []BehindQuery{{Repo: repo, Head: "abc", Base: behindBase}, {Repo: repo, Head: commitSHA, Base: "main"}} {
		if _, err := s.Behind(t.Context(), q); !errors.Is(err, errNotSHA) {
			t.Errorf("Behind(%+v) = %v, want errNotSHA", q, err)
		}
	}
	if n.Load() != 0 {
		t.Errorf("requests = %d, want 0", n.Load())
	}
}

func TestBehindShelfHit(t *testing.T) {
	api, n := behindAPI(nil)
	store := openStore(t)
	if _, err := New(api, WithStore(store)).Behind(t.Context(), behindQ); err != nil {
		t.Fatalf("Behind: %v", err)
	}

	// A new session reads what the shelf kept, however old, without GitHub.
	s := New(api, WithStore(cachetest.Aged(store, 30*24*time.Hour)))
	b, err := s.Behind(t.Context(), behindQ)
	if err != nil || b.AheadBy != 250 || len(b.Commits) != 2 {
		t.Fatalf("read in a new session = %+v, %v; want the kept commits", b, err)
	}
	if n.Load() != 1 {
		t.Errorf("requests = %d, want 1: the shelf answers", n.Load())
	}
}
