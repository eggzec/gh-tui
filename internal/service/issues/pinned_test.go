package issues

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
)

// pinnedFake answers PinnedIssues with issues 7 and 8.
func pinnedFake(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{t: t, pinnedIssues: func() ([]core.Issue, error) {
		return []core.Issue{issue(7), issue(8)}, nil
	}}
}

func TestPinnedReadsAndCaches(t *testing.T) {
	t.Parallel()
	api := pinnedFake(t)
	s := New(api)

	if _, ok := s.CachedPinned(repo); ok {
		t.Fatal("CachedPinned before a read reported a hit")
	}
	got, err := s.Pinned(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []core.Issue{issue(7), issue(8)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Pinned = %v, want %v", got, want)
	}
	api.checkCalls(t, "PinnedIssues")

	cached, ok := s.CachedPinned(repo)
	if !ok || !reflect.DeepEqual(cached, want) {
		t.Errorf("CachedPinned = %v, %v, want %v", cached, ok, want)
	}
	again, err := s.Pinned(t.Context(), repo)
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Errorf("second Pinned = %v, %v", again, err)
	}
	api.checkCalls(t)

	// What the caller gets is its own copy.
	got[0].Title = "changed by the caller"
	cached[1].Title = "changed by the caller"
	if again, _ := s.CachedPinned(repo); !reflect.DeepEqual(again, want) {
		t.Errorf("the cache shares its slice with a caller: %v", again)
	}
	if _, st := s.pinned.Get(pinnedKey(repo)); st == cache.Miss {
		t.Error("the pinned issues aren't under their key")
	}
}

func TestPinnedErrorCachesNothing(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	api := &fakeAPI{t: t, pinnedIssues: func() ([]core.Issue, error) { return nil, boom }}
	s := New(api)

	if _, err := s.Pinned(t.Context(), repo); !errors.Is(err, boom) {
		t.Fatalf("Pinned error = %v, want %v", err, boom)
	}
	if _, ok := s.CachedPinned(repo); ok {
		t.Error("a failed read cached something")
	}
}

func TestPinnedDroppedBySetStateAndInvalidate(t *testing.T) {
	t.Parallel()
	api := pinnedFake(t)
	s := New(api)
	api.setState = func(number int, state core.State) (core.Issue, error) {
		it := issue(number)
		it.State = state
		return it, nil
	}
	read := func() {
		t.Helper()
		if _, err := s.Pinned(t.Context(), repo); err != nil {
			t.Fatal(err)
		}
	}

	read()
	api.checkCalls(t, "PinnedIssues")
	if err := s.Close(repo, 7).Do(t.Context()); err != nil {
		t.Fatal(err)
	}
	api.checkCalls(t, "SetIssueState")
	read()
	api.checkCalls(t, "PinnedIssues")

	read()
	api.checkCalls(t)
	s.Invalidate(repo)
	read()
	api.checkCalls(t, "PinnedIssues")
}

func TestPinnedRollsBack(t *testing.T) {
	t.Parallel()
	api := pinnedFake(t)
	s := New(api)
	conflict := fmt.Errorf("github: 422: %w", core.ErrConflict)
	api.setState = func(int, core.State) (core.Issue, error) { return core.Issue{}, conflict }

	if _, err := s.Pinned(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	before, _ := s.CachedPinned(repo)
	op := s.Close(repo, 7)
	shown, _ := s.CachedPinned(repo)
	if reflect.DeepEqual(shown, before) {
		t.Error("the change isn't shown before it is sent")
	}
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("a refused change reported no error")
	}
	after, ok := s.CachedPinned(repo)
	if !ok || !reflect.DeepEqual(after, before) {
		t.Errorf("after the rollback = %v, %v, want %v", after, ok, before)
	}
}

func TestPinnedIsMemoryOnly(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	api := pinnedFake(t)
	first := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	if _, err := first.Pinned(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	api.checkCalls(t, "PinnedIssues")

	second := New(api, WithStore(store))
	if _, ok := second.CachedPinned(repo); ok {
		t.Error("a new session found the pinned issues of the old one")
	}
	if kept := second.Kept(); len(kept) != 0 {
		t.Errorf("the service keeps %d entries to revalidate, want 0", len(kept))
	}
}
