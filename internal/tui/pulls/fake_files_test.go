package pulls

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// fakeCommits are the commits a fakeService serves for every pull request, and
// the heads it was asked for.
type fakeCommits struct {
	commits core.PullCommits
	err     error
	reads   []pulls.PullCommitsQuery
}

// fakeRange is what a fakeService serves for the files of a range of commits,
// and the ranges it was asked for.
type fakeRange struct {
	// detail is served for every range, or the one in byBase when the base
	// of the range is there.
	detail core.CompareDetail
	byBase map[string]core.CompareDetail
	err    error
	reads  []pulls.RangeQuery
	ctxs   []context.Context
	// gate, when set, holds every read until it is closed or the read's
	// context is done.
	gate chan struct{}
	// cached makes CachedRange serve the ranges that were read.
	cached bool
	done   map[pulls.RangeQuery]bool
}

// RangeFiles serves the range that was set.
func (f *fakeService) RangeFiles(ctx context.Context, q pulls.RangeQuery) (core.CompareDetail, error) {
	f.mu.Lock()
	r := f.rangeFiles
	if r == nil {
		f.mu.Unlock()
		return core.CompareDetail{}, nil
	}
	r.reads = append(r.reads, q)
	r.ctxs = append(r.ctxs, ctx)
	gate := r.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return core.CompareDetail{}, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.err != nil {
		return core.CompareDetail{}, r.err
	}
	if r.cached {
		if r.done == nil {
			r.done = map[pulls.RangeQuery]bool{}
		}
		r.done[q] = true
	}
	return r.answer(q), nil
}

// CachedRange serves a range that was read, when cached is set.
func (f *fakeService) CachedRange(q pulls.RangeQuery) (core.CompareDetail, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r := f.rangeFiles; r != nil && r.cached && r.done[q] {
		return r.answer(q), true
	}
	return core.CompareDetail{}, false
}

func (r *fakeRange) answer(q pulls.RangeQuery) core.CompareDetail {
	if d, ok := r.byBase[q.Base]; ok {
		return d
	}
	return r.detail
}

// PullCommits serves the commits that were set.
func (f *fakeService) PullCommits(_ context.Context, q pulls.PullCommitsQuery) (core.PullCommits, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pullCommits == nil {
		return core.PullCommits{}, nil
	}
	f.pullCommits.reads = append(f.pullCommits.reads, q)
	return f.pullCommits.commits, f.pullCommits.err
}

// CachedPullCommits has none: the first read is the one that fetches.
func (f *fakeService) CachedPullCommits(pulls.PullCommitsQuery) (core.PullCommits, bool) {
	return core.PullCommits{}, false
}
