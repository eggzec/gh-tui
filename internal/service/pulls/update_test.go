package pulls

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestUpdateBranchSendsHeadAndMarksStale(t *testing.T) {
	api := &fakeAPI{}
	s := seeded(t, api, func(*core.PullRequest) {})
	before := take(t, s)

	op := s.UpdateBranch(repo, 1, "9f1c2e4")
	// Nothing shows before GitHub accepts it.
	if after := take(t, s); !reflect.DeepEqual(after, before) {
		t.Errorf("the update changed the cache before it was sent:\n%+v\nwant\n%+v", after, before)
	}
	if n := api.count("updatebranch"); n != 0 {
		t.Fatalf("sent %d updates before Do, want 0", n)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []string{"1@9f1c2e4"}; !slices.Equal(api.updates, want) {
		t.Errorf("updates = %v, want %v", api.updates, want)
	}
	// What was cached is still served, but each read goes to GitHub.
	if got := take(t, s); !reflect.DeepEqual(got, before) {
		t.Errorf("the stale entries were dropped:\n%+v\nwant\n%+v", got, before)
	}
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n := api.count("get"); n != 2 {
		t.Errorf("get called %d times, want 2: the detail must be stale", n)
	}
	if _, err := s.List(t.Context(), openFirst); err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := api.count("list"); n != 3 {
		t.Errorf("list called %d times, want 3: pages must be stale", n)
	}
}

func TestUpdateBranchMarksTimelineStale(t *testing.T) {
	f := twoTimelinePages()
	api := &fakeAPI{timeline: f.read}
	s := seeded(t, api, func(*core.PullRequest) {})
	q := TimelineQuery{Repo: repo, Number: 1}
	read := func() {
		t.Helper()
		if _, err := s.Timeline(t.Context(), q); err != nil {
			t.Fatalf("Timeline: %v", err)
		}
	}
	read()
	read()
	if n := api.count("timeline"); n != 1 {
		t.Fatalf("timeline read %d times, want 1: the second read is fresh", n)
	}

	if err := s.UpdateBranch(repo, 1, "9f1c2e4").Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, ok := s.CachedTimeline(q); !ok {
		t.Error("the stale timeline was dropped, want it kept until the next read")
	}
	read()
	if n := api.count("timeline"); n != 2 {
		t.Errorf("timeline read %d times, want 2: the update must make it stale", n)
	}
}

// TestUpdateBranchDropsKeptCopies checks what the next session finds on disk:
// an accepted update drops the kept detail and timeline of the old head, and a
// refused one leaves them.
func TestUpdateBranchDropsKeptCopies(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(fmt.Sprintf("accepted=%v", accepted), func(t *testing.T) {
			v := &versioned{updated: epoch, checks: core.ChecksSuccess}
			store := openStore(t)
			q := TimelineQuery{Repo: repo, Number: 1}
			api := v.api()
			api.timeline = twoTimelinePages().read
			if !accepted {
				api.updateErr = &core.RefusedError{Action: "branch can't be updated", Reason: "merge conflict", Err: core.ErrConflict}
			}
			s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
			readDetail(t, s)
			if _, err := s.Timeline(t.Context(), q); err != nil {
				t.Fatalf("Timeline: %v", err)
			}

			err := s.UpdateBranch(repo, 1, "9f1c2e4").Do(t.Context())
			if (err == nil) != accepted {
				t.Fatalf("Do = %v, accepted %v", err, accepted)
			}

			next := New(v.api(), WithStore(cachetest.Aged(store, time.Hour)))
			if _, ok := next.keptDetails.Load(detailKey(repo, 1)); ok != !accepted {
				t.Errorf("kept detail = %v, want %v", ok, !accepted)
			}
			if _, ok := next.keptTimelines.Load(q.key(next.pageSize)); ok != !accepted {
				t.Errorf("kept timeline = %v, want %v", ok, !accepted)
			}
		})
	}
}

func TestUpdateBranchRefusedByGitHub(t *testing.T) {
	for _, status := range []error{core.ErrConflict, core.ErrForbidden} {
		t.Run(status.Error(), func(t *testing.T) {
			refusal := &core.RefusedError{Action: "branch can't be updated", Reason: "merge conflict", Err: status}
			api := &fakeAPI{updateErr: refusal}
			s := seeded(t, api, func(*core.PullRequest) {})
			before := take(t, s)

			err := s.UpdateBranch(repo, 1, "9f1c2e4").Do(t.Context())
			got, ok := errors.AsType[*core.RefusedError](err)
			if !ok || got.Reason != "merge conflict" {
				t.Fatalf("Do = %v, want the RefusedError with GitHub's reason", err)
			}
			if !errors.Is(err, status) {
				t.Errorf("Do = %v, want it to match %v", err, status)
			}
			if after := take(t, s); !reflect.DeepEqual(after, before) {
				t.Errorf("a refused update changed the cache:\n%+v\nwant\n%+v", after, before)
			}
			// Nothing was marked stale, so the detail is still fresh.
			if _, err := s.Get(t.Context(), repo, 1); err != nil {
				t.Fatalf("Get: %v", err)
			}
			if n := api.count("get"); n != 1 {
				t.Errorf("get called %d times, want 1: a refused update leaves the detail fresh", n)
			}
		})
	}
}

func TestUpdateBranchRefusedByToken(t *testing.T) {
	api := &fakeAPI{}
	s := seeded(t, api, func(*core.PullRequest) {})
	s.access, s.repos = tokenWith(classic("gist"), true), nil
	sent := api.callsSoFar()

	err := s.UpdateBranch(repo, 1, "9f1c2e4").Do(t.Context())
	if _, ok := errors.AsType[*core.ScopeError](err); !ok {
		t.Fatalf("Do = %v, want a ScopeError", err)
	}
	if got := api.callsSoFar(); !maps.Equal(got, sent) {
		t.Errorf("calls = %v, want %v: nothing sent", got, sent)
	}
}
