package issues

import (
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestTimelineCachesPages(t *testing.T) {
	api := &fakeAPI{t: t, timeline: func(number int, cursor string, first int) (core.Page[core.TimelineItem], error) {
		if number != 7 || cursor != "C1" || first != 50 {
			t.Errorf("read (%d, %q, %d), want (7, C1, 50)", number, cursor, first)
		}
		return core.Page[core.TimelineItem]{Items: []core.TimelineItem{{Kind: core.TimelineClosed}}, Next: "C2"}, nil
	}}
	s := New(api)
	q := TimelineQuery{Repo: repo, Number: 7, Cursor: "C1", PageSize: 50}
	if _, ok := s.CachedTimeline(q); ok {
		t.Fatal("cached before any read")
	}
	p, err := s.Timeline(t.Context(), q)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(p.Items) != 1 || p.Next != "C2" {
		t.Errorf("page = %+v", p)
	}
	if _, ok := s.CachedTimeline(q); !ok {
		t.Error("page not cached after the read")
	}
	if _, err := s.Timeline(t.Context(), q); err != nil {
		t.Fatalf("second Timeline: %v", err)
	}
	api.checkCalls(t, "ListIssueTimeline")
}

func TestTimelineUsesServicePageSize(t *testing.T) {
	api := &fakeAPI{t: t, timeline: func(_ int, _ string, first int) (core.Page[core.TimelineItem], error) {
		if first != s0PageSize(t) {
			t.Errorf("first = %d, want the default", first)
		}
		return core.Page[core.TimelineItem]{}, nil
	}}
	s := New(api)
	if _, err := s.Timeline(t.Context(), TimelineQuery{Repo: repo, Number: 1}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.CachedTimeline(TimelineQuery{Repo: repo, Number: 1, PageSize: s0PageSize(t)}); !ok {
		t.Error("default and explicit page size don't share a key")
	}
}

func s0PageSize(t *testing.T) int {
	t.Helper()
	return New(&fakeAPI{t: t}).pageSize
}

func TestTimelineError(t *testing.T) {
	boom := errors.New("boom")
	api := &fakeAPI{t: t, timeline: func(int, string, int) (core.Page[core.TimelineItem], error) {
		return core.Page[core.TimelineItem]{}, boom
	}}
	_, err := New(api).Timeline(t.Context(), TimelineQuery{Repo: repo, Number: 3})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom wrapped", err)
	}
}
