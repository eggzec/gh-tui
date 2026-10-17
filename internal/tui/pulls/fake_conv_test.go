package pulls

import (
	"context"
	"slices"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// timelineFake is what the fake service serves of the timeline, and what it
// was asked, under the fake's lock.
type timelineFake struct {
	// pages are the pages Timeline serves by the cursor that reads them, ""
	// for the first, and cache those CachedTimeline serves.
	pages map[string]core.Page[core.TimelineItem]
	cache map[string]core.Page[core.TimelineItem]
	// reads are the reads of Timeline, and err fails them, as long as it is
	// set.
	reads []pulls.TimelineQuery
	err   error
	// gate, when set, holds every read until it is closed or the read's
	// context ends.
	gate chan struct{}
}

// Timeline serves the page of timeline.pages at the cursor, or an empty one.
func (f *fakeService) Timeline(ctx context.Context, q pulls.TimelineQuery) (core.Page[core.TimelineItem], error) {
	f.mu.Lock()
	gate := f.timeline.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return core.Page[core.TimelineItem]{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timeline.reads = append(f.timeline.reads, q)
	if f.timeline.err != nil {
		return core.Page[core.TimelineItem]{}, f.timeline.err
	}
	return f.timeline.pages[q.Cursor], nil
}

// CachedTimeline serves the page of timeline.cache at the cursor, if there is
// one.
func (f *fakeService) CachedTimeline(q pulls.TimelineQuery) (core.Page[core.TimelineItem], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.timeline.cache[q.Cursor]
	return p, ok
}

// timelineReads returns the reads of Timeline so far.
func (f *fakeService) timelineReads() []pulls.TimelineQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.timeline.reads)
}

// threadReadsSoFar returns the reads of Threads so far.
func (f *fakeService) threadReadsSoFar() []pulls.ThreadsQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.threadReads)
}
