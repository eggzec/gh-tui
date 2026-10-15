package pulls

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

const (
	oldHead = "1111111111111111111111111111111111111111"
	newHead = "2222222222222222222222222222222222222222"
)

var (
	sinceQ = SinceQuery{Repo: repo, Base: oldHead, Head: newHead}
	rangeQ = RangeQuery{Repo: repo, Base: oldHead, Head: newHead}
)

func TestSinceETagAfterTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var conds []string
		api := &fakeAPI{compare: func(_ context.Context, _ core.RepoRef, base, head string, cond github.Conditional) (core.Compare, github.Response, error) {
			if base != oldHead || head != newHead {
				t.Errorf("compared %s...%s", base, head)
			}
			conds = append(conds, cond.ETag)
			if cond.ETag == `W/"c1"` {
				return core.Compare{}, github.Response{NotModified: true}, nil
			}
			return core.Compare{Status: core.CompareAhead, AheadBy: 3}, github.Response{ETag: `W/"c1"`}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		if _, ok := s.CachedSince(sinceQ); ok {
			t.Fatal("CachedSince before a read = true")
		}
		want := core.Compare{Status: core.CompareAhead, AheadBy: 3}
		for range 2 {
			got, err := s.Since(t.Context(), sinceQ)
			if err != nil || got != want {
				t.Fatalf("Since = %+v, %v; want %+v", got, err, want)
			}
		}
		if len(conds) != 1 {
			t.Fatalf("requests = %q, want one: a fresh comparison is a hit", conds)
		}
		time.Sleep(2 * time.Minute)
		got, err := s.Since(t.Context(), sinceQ)
		if err != nil || got != want {
			t.Fatalf("Since after the TTL = %+v, %v; want %+v", got, err, want)
		}
		if len(conds) != 2 || conds[1] != `W/"c1"` {
			t.Errorf("requests = %q, want the second to carry the ETag", conds)
		}
		if _, ok := s.CachedSince(sinceQ); !ok {
			t.Error("CachedSince after a read = false")
		}
	})
}

func TestSinceServesStaleOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var fail bool
		api := &fakeAPI{compare: func(context.Context, core.RepoRef, string, string, github.Conditional) (core.Compare, github.Response, error) {
			if fail {
				return core.Compare{}, github.Response{}, errDial
			}
			return core.Compare{Status: core.CompareDiverged, AheadBy: 1, BehindBy: 2}, github.Response{ETag: `W/"c1"`}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Since(t.Context(), sinceQ); err != nil {
			t.Fatal(err)
		}
		fail = true
		time.Sleep(2 * time.Minute)
		got, err := s.Since(t.Context(), sinceQ)
		if err != nil || got.AheadBy != 1 || got.BehindBy != 2 {
			t.Errorf("Since offline = %+v, %v; want the stale comparison", got, err)
		}
	})
}

func TestRewritten(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusUnprocessableEntity} {
		cause := &github.Error{StatusCode: status}
		api := &fakeAPI{
			compare: func(context.Context, core.RepoRef, string, string, github.Conditional) (core.Compare, github.Response, error) {
				return core.Compare{}, github.Response{}, cause
			},
			compareFiles: func(context.Context, core.RepoRef, string, string, github.Conditional) (core.CompareDetail, github.Response, error) {
				return core.CompareDetail{}, github.Response{}, cause
			},
		}
		s := New(api)
		_, err := s.Since(t.Context(), sinceQ)
		if !errors.Is(err, core.ErrRewritten) || !errors.Is(err, cause) {
			t.Errorf("Since with %d: err = %v, want core.ErrRewritten wrapping it", status, err)
		}
		_, err = s.RangeFiles(t.Context(), rangeQ)
		if !errors.Is(err, core.ErrRewritten) || !errors.Is(err, cause) {
			t.Errorf("RangeFiles with %d: err = %v, want core.ErrRewritten wrapping it", status, err)
		}
	}
}

func TestRewrittenLeavesOtherErrors(t *testing.T) {
	cause := &github.Error{StatusCode: http.StatusBadGateway}
	api := &fakeAPI{
		compare: func(context.Context, core.RepoRef, string, string, github.Conditional) (core.Compare, github.Response, error) {
			return core.Compare{}, github.Response{}, cause
		},
		compareFiles: func(context.Context, core.RepoRef, string, string, github.Conditional) (core.CompareDetail, github.Response, error) {
			return core.CompareDetail{}, github.Response{}, cause
		},
	}
	s := New(api)
	if _, err := s.Since(t.Context(), sinceQ); errors.Is(err, core.ErrRewritten) || !errors.Is(err, core.ErrUnavailable) {
		t.Errorf("Since with a 502: err = %v, want it not rewritten", err)
	}
	if _, err := s.RangeFiles(t.Context(), rangeQ); errors.Is(err, core.ErrRewritten) || !errors.Is(err, core.ErrUnavailable) {
		t.Errorf("RangeFiles with a 502: err = %v, want it not rewritten", err)
	}
}

// ranged serves one range of two files, and counts the reads.
func ranged() *fakeAPI {
	return &fakeAPI{compareFiles: func(_ context.Context, _ core.RepoRef, _, _ string, cond github.Conditional) (core.CompareDetail, github.Response, error) {
		if cond != (github.Conditional{}) {
			return core.CompareDetail{}, github.Response{}, errors.New("a range by SHAs was asked for conditionally")
		}
		return core.CompareDetail{
			Status: core.CompareAhead, AheadBy: 2,
			TotalCommits: 2,
			Files: []core.CommitFile{
				{Path: "a.go", Status: core.FileModified, SHA: "b1", Additions: 1, Patch: "@@ -1 +1 @@\n-a\n+b"},
				{Path: "b.go", PreviousPath: "c.go", Status: core.FileRenamed, SHA: "b2"},
			},
			Truncated: true,
		}, github.Response{ETag: `W/"r"`}, nil
	}}
}

func TestRangeFilesReadOnce(t *testing.T) {
	api := ranged()
	s := New(api)
	if _, ok := s.CachedRange(rangeQ); ok {
		t.Fatal("CachedRange before a read = true")
	}
	for range 2 {
		d, err := s.RangeFiles(t.Context(), rangeQ)
		if err != nil {
			t.Fatalf("RangeFiles: %v", err)
		}
		if len(d.Files) != 2 || d.Files[0].SHA != "b1" || d.Files[1].PreviousPath != "c.go" || d.TotalCommits != 2 || d.AheadBy != 2 || !d.Truncated {
			t.Fatalf("range = %+v, want both files with their blobs, the counts and Truncated", d)
		}
	}
	if n := api.count("comparefiles"); n != 1 {
		t.Errorf("requests = %d, want 1: the files between two SHAs don't go stale", n)
	}
	if _, ok := s.CachedRange(rangeQ); !ok {
		t.Error("CachedRange after a read = false")
	}
	other := rangeQ
	other.Head = "3333333333333333333333333333333333333333"
	if _, ok := s.CachedRange(other); ok {
		t.Error("CachedRange of another head = true")
	}
}

func TestRangeFilesAfterTTLStillHit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := ranged()
		s := New(api, WithTTL(time.Minute))
		if _, err := s.RangeFiles(t.Context(), rangeQ); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Hour)
		if _, err := s.RangeFiles(t.Context(), rangeQ); err != nil {
			t.Fatal(err)
		}
		if n := api.count("comparefiles"); n != 1 {
			t.Errorf("requests = %d, want 1", n)
		}
	})
}

func TestRangeFilesShelf(t *testing.T) {
	store := openStore(t)
	first := ranged()
	if _, err := New(first, WithStore(store)).RangeFiles(t.Context(), rangeQ); err != nil {
		t.Fatal(err)
	}

	// A later session reads the range from the store, without a request.
	second := ranged()
	d, err := New(second, WithStore(store)).RangeFiles(t.Context(), rangeQ)
	if err != nil {
		t.Fatalf("RangeFiles from the store: %v", err)
	}
	if n := second.count("comparefiles"); n != 0 {
		t.Errorf("requests = %d, want 0: the shelf has the range", n)
	}
	if len(d.Files) != 2 || d.Files[0].Patch == "" || !d.Truncated || d.TotalCommits != 2 {
		t.Errorf("range = %+v, want what the first session read", d)
	}
}

func TestRangeFilesNeedsSHAs(t *testing.T) {
	api := ranged()
	s := New(api)
	for _, q := range []RangeQuery{
		{Repo: repo, Base: "main", Head: newHead},
		{Repo: repo, Base: oldHead, Head: "feat/x"},
		{Repo: repo, Base: oldHead[:7], Head: newHead},
		{Repo: repo},
	} {
		if _, err := s.RangeFiles(t.Context(), q); !errors.Is(err, errNotSHA) {
			t.Errorf("RangeFiles(%q, %q) err = %v, want errNotSHA", q.Base, q.Head, err)
		}
	}
	if n := api.count("comparefiles"); n != 0 {
		t.Errorf("requests = %d, want 0", n)
	}
}
