package pulls

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func viewedRead() *fakeAPI {
	api := &fakeAPI{id: func(context.Context, core.RepoRef, int) (string, error) { return "PR_1", nil }}
	api.viewedRead = core.PullViewed{
		States:    map[string]core.ViewedState{"a.go": core.Viewed, "b.go": core.ViewedChanged},
		Supported: true,
	}
	return api
}

var viewedQ = ViewedQuery{Repo: repo, Number: 1, Head: "AAA"}

func TestViewedIsCachedByHead(t *testing.T) {
	api := viewedRead()
	s := New(api)
	if _, ok := s.CachedViewed(viewedQ); ok {
		t.Fatal("marks are cached before they were read")
	}
	for range 2 {
		got, err := s.Viewed(t.Context(), viewedQ)
		if err != nil || !reflect.DeepEqual(got, api.viewedRead) {
			t.Fatalf("Viewed = %+v, %v; want the marks", got, err)
		}
	}
	if n := api.reads(); n != 1 {
		t.Errorf("reads = %d, want 1: the second was cached", n)
	}
	if _, ok := s.CachedViewed(ViewedQuery{Repo: repo, Number: 1, Head: "aaa"}); !ok {
		t.Error("the head's case changes the key")
	}
	if _, ok := s.CachedViewed(ViewedQuery{Repo: repo, Number: 1, Head: "bbb"}); ok {
		t.Error("another head is cached: a push must read the marks anew")
	}
}

func TestViewedNeedsAHead(t *testing.T) {
	api := viewedRead()
	s := New(api)
	q := ViewedQuery{Repo: repo, Number: 1}
	if _, err := s.Viewed(t.Context(), q); !errors.Is(err, errNoHead) {
		t.Errorf("Viewed err = %v, want errNoHead", err)
	}
	if _, ok := s.CachedViewed(q); ok {
		t.Error("CachedViewed without a head reports a hit")
	}
	if n := api.reads(); n != 0 {
		t.Errorf("reads = %d, want 0", n)
	}
}

func TestViewedReadAgainAfterARefresh(t *testing.T) {
	for name, refresh := range map[string]func(s *Service){
		"a refresh of the repository": func(s *Service) { s.Invalidate(repo) },
		"a change a probe found":      func(s *Service) { s.invalidate(repo) },
	} {
		t.Run(name, func(t *testing.T) {
			api := viewedRead()
			s := New(api)
			if _, err := s.Viewed(t.Context(), viewedQ); err != nil {
				t.Fatal(err)
			}
			refresh(s)
			if _, err := s.Viewed(t.Context(), viewedQ); err != nil {
				t.Fatal(err)
			}
			if n := api.reads(); n != 2 {
				t.Errorf("reads = %d, want 2: the refresh drops the marks", n)
			}
		})
	}
}

func TestSetViewedShowsAtOnceAndSends(t *testing.T) {
	api := viewedRead()
	s := New(api)
	if _, err := s.Viewed(t.Context(), viewedQ); err != nil {
		t.Fatal(err)
	}
	op := s.SetViewed(viewedQ, "c.go", true)
	got, _ := s.CachedViewed(viewedQ)
	if got.State("c.go") != core.Viewed || got.State("a.go") != core.Viewed {
		t.Errorf("marks = %+v, want c.go marked at once and a.go kept", got.States)
	}
	if n := len(api.viewedMarks()); n != 0 {
		t.Errorf("sent %d before Do, want 0: SetViewed does no I/O", n)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []string{"c.go=true"}; !reflect.DeepEqual(api.viewedMarks(), want) {
		t.Errorf("sent = %v, want %v", api.viewedMarks(), want)
	}
	if got, _ := s.CachedViewed(viewedQ); got.State("c.go") != core.Viewed {
		t.Error("the mark is gone after it was accepted")
	}

	// An unmark drops the mark, of a file that changed since too.
	if err := s.SetViewed(viewedQ, "b.go", false).Do(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, _ = s.CachedViewed(viewedQ)
	if _, ok := got.States["b.go"]; ok || got.State("b.go") != core.Unviewed {
		t.Errorf("marks = %+v, want b.go unmarked", got.States)
	}
}

func TestSetViewedRollsBackWhenRefused(t *testing.T) {
	api := viewedRead()
	api.viewedErr = errors.New("nope")
	s := New(api)
	if _, err := s.Viewed(t.Context(), viewedQ); err != nil {
		t.Fatal(err)
	}
	err := s.SetViewed(viewedQ, "c.go", true).Do(t.Context())
	if err == nil || !errors.Is(err, api.viewedErr) {
		t.Fatalf("Do = %v, want the refusal", err)
	}
	got, _ := s.CachedViewed(viewedQ)
	if !reflect.DeepEqual(got, api.viewedRead) {
		t.Errorf("marks = %+v, want %+v back as before", got, api.viewedRead)
	}
}

func TestSetViewedLeavesTheSnapshotAlone(t *testing.T) {
	api := viewedRead()
	s := New(api)
	before, err := s.Viewed(t.Context(), viewedQ)
	if err != nil {
		t.Fatal(err)
	}
	s.SetViewed(viewedQ, "c.go", true)
	s.SetViewed(viewedQ, "a.go", false)
	if _, ok := before.States["c.go"]; ok || before.State("a.go") != core.Viewed {
		t.Errorf("the snapshot taken before changed: %+v", before.States)
	}
}

func TestSetViewedWithoutCachedMarksStillSends(t *testing.T) {
	api := viewedRead()
	s := New(api)
	if err := s.SetViewed(viewedQ, "c.go", true).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []string{"c.go=true"}; !reflect.DeepEqual(api.viewedMarks(), want) {
		t.Errorf("sent = %v, want %v", api.viewedMarks(), want)
	}
	if _, ok := s.CachedViewed(viewedQ); ok {
		t.Error("an unread entry was made by the change")
	}
}
