package pulls

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// timelineAPIFake is the API of timeline reads: pages by cursor, a failure to
// answer with, and the cursors it was asked for.
type timelineAPIFake struct {
	mu      sync.Mutex
	pages   map[string]core.Page[core.TimelineItem]
	err     error
	cursors []string
}

func (f *timelineAPIFake) read(cursor string) (core.Page[core.TimelineItem], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cursors = append(f.cursors, cursor)
	if f.err != nil {
		return core.Page[core.TimelineItem]{}, f.err
	}
	return f.pages[cursor], nil
}

// ListPullRequestTimeline makes the fake API a pulls.API, as the tests of the
// other reads do with their functions.
func (f *fakeAPI) ListPullRequestTimeline(_ context.Context, _ core.RepoRef, _ int, cursor string, _ int) (core.Page[core.TimelineItem], error) {
	f.called("timeline")
	return f.timeline(cursor)
}

var timelineQ = TimelineQuery{Repo: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, Number: 42}

func twoTimelinePages() *timelineAPIFake {
	return &timelineAPIFake{pages: map[string]core.Page[core.TimelineItem]{
		"":   {Items: []core.TimelineItem{{Kind: core.TimelineComment, ID: "a"}, {Kind: core.TimelineLabeled}}, Next: "c1"},
		"c1": {Items: []core.TimelineItem{{Kind: core.TimelineMerged}}},
	}}
}

func TestTimelineReadsEveryPage(t *testing.T) {
	f := twoTimelinePages()
	s := New(&fakeAPI{timeline: f.read})
	p, err := s.Timeline(t.Context(), timelineQ)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(p.Items) != 3 || p.Items[0].ID != "a" || p.Items[2].Kind != core.TimelineMerged || p.Next != "" {
		t.Errorf("timeline = %+v, want the three items of both pages in order, with no Next", p)
	}
	if fmt.Sprint(f.cursors) != "[ c1]" {
		t.Errorf("cursors = %q, want the first page and the page after c1", f.cursors)
	}
	if _, ok := s.CachedTimeline(timelineQ); !ok {
		t.Error("CachedTimeline after a read = false")
	}
	if _, err := s.Timeline(t.Context(), timelineQ); err != nil || len(f.cursors) != 2 {
		t.Errorf("second read: err %v, cursors %q, want a cache hit", err, f.cursors)
	}
}

func TestTimelineStopsAtACursorThatDoesNotMove(t *testing.T) {
	f := &timelineAPIFake{pages: map[string]core.Page[core.TimelineItem]{
		"":   {Items: []core.TimelineItem{{ID: "a"}}, Next: "c1"},
		"c1": {Items: []core.TimelineItem{{ID: "b"}}, Next: "c1"},
	}}
	s := New(&fakeAPI{timeline: f.read})
	p, err := s.Timeline(t.Context(), timelineQ)
	if err != nil || len(p.Items) != 2 {
		t.Fatalf("Timeline = %+v, %v; want two items and no endless read", p, err)
	}
}

func TestTimelineFailedPageFailsTheRead(t *testing.T) {
	f := twoTimelinePages()
	f.err = &core.RateLimitError{}
	s := New(&fakeAPI{timeline: f.read})
	if _, err := s.Timeline(t.Context(), timelineQ); err == nil {
		t.Fatal("Timeline with a failing read = nil error")
	}
	if _, ok := s.CachedTimeline(timelineQ); ok {
		t.Error("CachedTimeline after a failed read = true")
	}
}

func TestTimelineServedWhenOfflineOrLimited(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		offline bool
	}{
		{"offline", errDial, true},
		{"limited", fmt.Errorf("graphql: %w", &core.RateLimitError{Reset: epoch}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := twoTimelinePages()
				s := New(&fakeAPI{timeline: f.read}, WithTTL(time.Minute))
				if _, err := s.Timeline(t.Context(), timelineQ); err != nil {
					t.Fatal(err)
				}
				time.Sleep(2 * time.Minute)
				f.mu.Lock()
				f.err = tt.err
				f.mu.Unlock()
				p, err := s.Timeline(t.Context(), timelineQ)
				if err != nil {
					t.Fatalf("Timeline: %v", err)
				}
				if len(p.Items) != 3 || p.Offline != tt.offline || p.Limited == tt.offline {
					t.Errorf("timeline = %+v, want the cached one, offline %v", p, tt.offline)
				}
			})
		})
	}
}
