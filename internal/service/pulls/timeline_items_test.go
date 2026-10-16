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

func TestTimelineReadsOnePagePerCursor(t *testing.T) {
	f := twoTimelinePages()
	s := New(&fakeAPI{timeline: f.read})
	second := timelineQ
	second.Cursor = "c1"

	p, err := s.Timeline(t.Context(), timelineQ)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(p.Items) != 2 || p.Items[0].ID != "a" || p.Next != "c1" {
		t.Errorf("first page = %+v, want its two items and Next c1", p)
	}
	if fmt.Sprint(f.cursors) != "[]" {
		t.Errorf("cursors = %q, want only the first page read", f.cursors)
	}
	if _, ok := s.CachedTimeline(second); ok {
		t.Error("CachedTimeline of the second page before it was read = true")
	}

	p, err = s.Timeline(t.Context(), second)
	if err != nil {
		t.Fatalf("Timeline of the second page: %v", err)
	}
	if len(p.Items) != 1 || p.Items[0].Kind != core.TimelineMerged || p.Next != "" {
		t.Errorf("second page = %+v, want the merged item and no Next", p)
	}
	for _, q := range []TimelineQuery{timelineQ, second} {
		if _, ok := s.CachedTimeline(q); !ok {
			t.Errorf("CachedTimeline(cursor %q) after a read = false", q.Cursor)
		}
		if _, err := s.Timeline(t.Context(), q); err != nil {
			t.Fatalf("Timeline again: %v", err)
		}
	}
	if fmt.Sprint(f.cursors) != "[ c1]" {
		t.Errorf("cursors = %q, want one read of each page and cache hits after", f.cursors)
	}
}

// timelineRequests reads the first page of the timeline and returns how many
// requests the API has had for it so far.
func timelineRequests(t *testing.T, s *Service, api *fakeAPI) int {
	t.Helper()
	if _, err := s.Timeline(t.Context(), timelineQ); err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	return api.count("timeline")
}

// timelineService is a service that has read the first page of the timeline,
// and the API it read it from, which also answers the changes the tests make.
func timelineService(t *testing.T, opts ...Option) (*Service, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{
		timeline: twoTimelinePages().read,
		id:       func(context.Context, core.RepoRef, int) (string, error) { return "PR_42", nil },
		mutate: func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
			return openPull(timelineQ.Number), nil
		},
	}
	s := New(api, opts...)
	if n := timelineRequests(t, s, api); n != 1 {
		t.Fatalf("requests after the first read = %d, want 1", n)
	}
	if n := timelineRequests(t, s, api); n != 1 {
		t.Fatalf("requests after a fresh read = %d, want a cache hit", n)
	}
	return s, api
}

func TestTimelineInvalidateReadsAgain(t *testing.T) {
	s, api := timelineService(t)
	s.Invalidate(timelineQ.Repo)
	if _, ok := s.CachedTimeline(timelineQ); !ok {
		t.Error("the timeline was dropped, want it kept stale")
	}
	if n := timelineRequests(t, s, api); n != 2 {
		t.Errorf("requests after Invalidate = %d, want 2", n)
	}
}

func TestTimelineChangedReadsAgain(t *testing.T) {
	s, api := timelineService(t)
	s.Changed(timelineQ.Repo, timelineQ.Number, time.Now().Add(-time.Hour))
	if n := timelineRequests(t, s, api); n != 1 {
		t.Errorf("requests after an older change = %d, want 1", n)
	}
	s.Changed(timelineQ.Repo, timelineQ.Number, time.Now().Add(time.Minute))
	if n := timelineRequests(t, s, api); n != 2 {
		t.Errorf("requests after a later change = %d, want 2", n)
	}
}

func TestTimelineSubmittedReviewReadsAgain(t *testing.T) {
	s, api := timelineService(t, WithStore(openStore(t)))
	err := s.SubmitReview(t.Context(), timelineQ.Repo, timelineQ.Number, "abc123", core.ReviewEventRequestChanges, "please fix", reviewDrafts)
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if n := timelineRequests(t, s, api); n != 2 {
		t.Errorf("requests after a review = %d, want 2", n)
	}
}

func TestTimelineChangeReadsAgain(t *testing.T) {
	s, api := timelineService(t, WithStore(openStore(t)))
	if err := s.Close(timelineQ.Repo, timelineQ.Number).Do(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := timelineRequests(t, s, api); n != 2 {
		t.Errorf("requests after closing = %d, want 2", n)
	}
}

func TestTimelineCommentReadsAgain(t *testing.T) {
	s, api := timelineService(t, WithStore(openStore(t)))
	if err := s.Comment(timelineQ.Repo, timelineQ.Number, "hi", "me").Do(t.Context()); err != nil {
		t.Fatalf("Comment: %v", err)
	}
	if n := timelineRequests(t, s, api); n != 2 {
		t.Errorf("requests after a comment = %d, want 2", n)
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
				if len(p.Items) != 2 || p.Offline != tt.offline || p.Limited == tt.offline {
					t.Errorf("timeline = %+v, want the cached one, offline %v", p, tt.offline)
				}
			})
		})
	}
}
