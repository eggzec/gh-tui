package pulls

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// CommitChecks is what the Merge pane needs of the actions service to show the
// checks of the head commit.
type CommitChecks interface {
	// CachedCommitChecks returns what is kept of the checks of q, and does no
	// I/O.
	CachedCommitChecks(q actions.CommitChecksQuery) (core.CommitChecks, bool)
	CommitChecks(ctx context.Context, q actions.CommitChecksQuery) (core.CommitChecks, error)
}

// Behinds is what the Merge pane needs to count the commits that a branch is
// behind its base. The pull requests service has them; a service that lacks
// them leaves the count unknown.
type Behinds interface {
	CachedBehind(q pulls.BehindQuery) (core.BaseCommits, bool)
	Behind(ctx context.Context, q pulls.BehindQuery) (core.BaseCommits, error)
}

// BranchUpdater is what the Merge pane needs to update a branch with its base.
// The returned Op changes nothing at once.
type BranchUpdater interface {
	UpdateBranch(repo core.RepoRef, number int, head string) *optimistic.Op
}

// WithCommitChecks gives the Merge pane the reader of the checks of a commit.
// Without it, the pane shows no checks.
func WithCommitChecks(r CommitChecks) Option {
	return func(s *Section) { s.commits = r }
}
