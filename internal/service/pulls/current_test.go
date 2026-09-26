package pulls

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// versioned is a fake GitHub whose pull request #1 was last updated at
// updated with checks in state checks. The list and the detail agree.
type versioned struct {
	mu      sync.Mutex
	updated time.Time
	checks  core.ChecksState
}

func (v *versioned) set(updated time.Time, checks core.ChecksState) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.updated, v.checks = updated, checks
}

func (v *versioned) pull() core.PullRequest {
	v.mu.Lock()
	defer v.mu.Unlock()
	pr := openPull(1)
	pr.UpdatedAt, pr.Checks = v.updated, v.checks
	return pr
}

func (v *versioned) api() *fakeAPI {
	return &fakeAPI{
		list: func(context.Context, core.RepoRef, core.State, string, int) (core.Page[core.PullRequest], error) {
			return core.Page[core.PullRequest]{Items: []core.PullRequest{v.pull()}}, nil
		},
		get: func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
			return core.PullRequestDetail{PullRequest: v.pull()}, nil
		},
		comments: func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Comment], error) {
			return core.Page[core.Comment]{Items: []core.Comment{{ID: "c"}}}, nil
		},
	}
}

var firstComments = CommentsQuery{Repo: repo, Number: 1}

// readDetail reads the detail and the first comments of #1.
func readDetail(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.Comments(t.Context(), firstComments); err != nil {
		t.Fatalf("Comments: %v", err)
	}
}

func list(t *testing.T, s *Service, q ListQuery) {
	t.Helper()
	if _, err := s.List(t.Context(), q); err != nil {
		t.Fatalf("List: %v", err)
	}
}

// relist reads q again after a read that served it kept, which asks
// GitHub.
func relist(t *testing.T, s *Service, q ListQuery) {
	t.Helper()
	if _, err := s.List(t.Context(), q.again()); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func wantCalls(t *testing.T, api *fakeAPI, get, comments int) {
	t.Helper()
	if n := api.count("get"); n != get {
		t.Errorf("get called %d times, want %d", n, get)
	}
	if n := api.count("comments"); n != comments {
		t.Errorf("comments called %d times, want %d", n, comments)
	}
}

func TestCurrentDetailOutlivesTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: clock, checks: core.ChecksSuccess}
		api := v.api()
		s := New(api, WithTTL(time.Minute))
		list(t, s, openFirst)
		readDetail(t, s)

		time.Sleep(time.Hour)
		if !s.Current(firstComments) {
			t.Error("Current = false past the TTL, want true: the list vouches for #1")
		}
		readDetail(t, s)
		wantCalls(t, api, 1, 1)
	})
}

func TestNewerVersionRefetches(t *testing.T) {
	v := &versioned{updated: clock, checks: core.ChecksSuccess}
	api := v.api()
	s := New(api)
	list(t, s, openFirst)
	readDetail(t, s)

	// Another list page, well within the TTL, shows that #1 changed.
	v.set(clock.Add(time.Minute), core.ChecksSuccess)
	list(t, s, ListQuery{Repo: repo, State: core.StateOpen, PageSize: 10})
	if s.Current(firstComments) {
		t.Error("Current = true after #1 changed, want false")
	}
	readDetail(t, s)
	wantCalls(t, api, 2, 2)
	if d, _ := s.CachedGet(repo, 1); !d.UpdatedAt.Equal(clock.Add(time.Minute)) {
		t.Errorf("cached detail updated at %v, want the new version", d.UpdatedAt)
	}
	// The new version is current in turn.
	readDetail(t, s)
	wantCalls(t, api, 2, 2)
}

func TestUnlistedDetailFollowsTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Opened from the search, no list vouches for #1.
		api := (&versioned{updated: clock, checks: core.ChecksSuccess}).api()
		s := New(api, WithTTL(time.Minute))
		readDetail(t, s)
		readDetail(t, s)
		wantCalls(t, api, 1, 1)

		time.Sleep(2 * time.Minute)
		if s.Current(firstComments) {
			t.Error("Current = true past the TTL without a list")
		}
		readDetail(t, s)
		wantCalls(t, api, 2, 2)
	})
}

func TestChecksKeepTheirTTL(t *testing.T) {
	t.Run("changed checks", func(t *testing.T) {
		v := &versioned{updated: clock, checks: core.ChecksPending}
		api := v.api()
		s := New(api)
		list(t, s, openFirst)
		readDetail(t, s)

		// Checks finishing don't move the update time.
		v.set(clock, core.ChecksFailure)
		list(t, s, ListQuery{Repo: repo, State: core.StateOpen, PageSize: 10})
		readDetail(t, s)
		// Only the detail holds the checks.
		wantCalls(t, api, 2, 1)
		if d, _ := s.CachedGet(repo, 1); d.Checks != core.ChecksFailure {
			t.Errorf("cached checks = %q, want failure", d.Checks)
		}
	})
	t.Run("running checks", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			api := (&versioned{updated: clock, checks: core.ChecksPending}).api()
			s := New(api, WithTTL(time.Minute))
			list(t, s, openFirst)
			readDetail(t, s)

			time.Sleep(2 * time.Minute)
			readDetail(t, s)
			// Running checks may change without the rollup state, so the
			// detail is read again. The comments are still current.
			wantCalls(t, api, 2, 1)
		})
	})
}

func TestChangeForgetsVersion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: clock, checks: core.ChecksSuccess}
		api := v.api()
		api.mutate = func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
			pr := v.pull()
			pr.State = core.StateClosed
			return pr, nil
		}
		s := New(api, WithTTL(time.Minute))
		list(t, s, openFirst)
		readDetail(t, s)
		if err := s.Close(repo, 1).Do(t.Context()); err != nil {
			t.Fatalf("Close: %v", err)
		}

		time.Sleep(2 * time.Minute)
		readDetail(t, s)
		wantCalls(t, api, 2, 2)
	})
}

func TestChangedMarksOlderStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: time.Now(), checks: core.ChecksSuccess}
		api := v.api()
		s := New(api, WithTTL(time.Hour))
		list(t, s, openFirst)
		readDetail(t, s)

		// A notification of a change the cache has seen changes nothing.
		s.Changed(repo, 1, time.Now().Add(-time.Second))
		if !s.Current(firstComments) {
			t.Error("Current = false after an older change, want true")
		}

		time.Sleep(time.Minute)
		changed := time.Now()
		v.set(changed, core.ChecksSuccess)
		s.Changed(repo, 1, changed)
		if s.Current(firstComments) {
			t.Error("Current = true after a newer change, want false")
		}
		readDetail(t, s)
		wantCalls(t, api, 2, 2)

		// What was read since is as recent as the change.
		s.Changed(repo, 1, changed)
		if !s.Current(firstComments) {
			t.Error("Current = false after reading the change, want true")
		}
	})
}

func TestChangedKeepsDetailThatShowsIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		changed := time.Now().Add(time.Second)
		// GitHub had the change before the read, as the detail shows.
		api := (&versioned{updated: changed, checks: core.ChecksSuccess}).api()
		s := New(api, WithTTL(time.Hour))
		readDetail(t, s)

		time.Sleep(time.Minute)
		s.Changed(repo, 1, changed)
		readDetail(t, s)
		// Only the comments, read before the change, are read again.
		wantCalls(t, api, 1, 2)
	})
}

func TestCurrentReadsCountAsHits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := obs.NewStats()
		prev := obs.SetDefault(stats)
		defer obs.SetDefault(prev)
		v := &versioned{updated: clock, checks: core.ChecksSuccess}
		s := New(v.api(), WithTTL(time.Minute))
		list(t, s, openFirst)
		readDetail(t, s)
		time.Sleep(time.Hour)
		// Past the TTL, the list vouches for them: two hits, no misses.
		readDetail(t, s)
		if !s.Current(firstComments) {
			t.Fatal("Current = false, want true")
		}
		var hit, miss int64
		for _, c := range stats.Summary().Cache {
			hit, miss = hit+c.Hit, miss+c.Miss
		}
		// The misses are the list, the detail and the comments.
		if hit != 2 || miss != 3 {
			t.Errorf("counted %d hits and %d misses, want 2 and 3", hit, miss)
		}
	})
}
