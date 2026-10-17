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
