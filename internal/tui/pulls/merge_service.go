package pulls

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
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

// CodeOwners is what the Merge pane needs to say who must review a pull
// request: the CODEOWNERS file of its base and the members of a team. The pull
// requests service has them; without a reader, the preview of the Review
// required row has no owners.
type CodeOwners interface {
	CachedOwners(q pulls.OwnersQuery) (pulls.OwnersFile, bool)
	Owners(ctx context.Context, q pulls.OwnersQuery) (pulls.OwnersFile, error)
	CachedTeamMembers(q pulls.TeamQuery) (pulls.TeamMembers, bool)
	TeamMembers(ctx context.Context, q pulls.TeamQuery) (pulls.TeamMembers, error)
}

// WithCodeOwners gives the Merge pane the reader of the code owners of a pull
// request.
func WithCodeOwners(r CodeOwners) Option {
	return func(s *Section) { s.codeOwners = r }
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

// CommitWatch starts the follow of the checks of a commit, which stores newer
// checks where [CommitChecks] reads them and tells the app with a ui.SyncMsg of
// the key actions.CommitChecksSyncKey gives. It returns what ends the follow.
type CommitWatch func(q actions.CommitChecksQuery) (stop func())

// WithCommitChecksWatch makes the Merge pane follow the checks of the head
// commit while some are pending. Without it, the pane reads them once.
func WithCommitChecksWatch(w CommitWatch) Option {
	return func(s *Section) { s.commitWatch = w }
}

// RunFollow starts the poll of a workflow run, which reads its jobs and the
// logs of those running, and tells the app with a ui.SyncMsg of the key
// actions.RunSyncKey gives. It returns what ends the poll.
type RunFollow func(repo core.RepoRef, runID int64) (stop func())

// WithRunFollow makes the Merge pane follow the run of a running job while the
// preview shows its log. Without it, the log is read once.
func WithRunFollow(f RunFollow) Option {
	return func(s *Section) { s.runFollow = f }
}

// commitReader adapts a [CommitChecks] to the reader of a checks tree.
type commitReader struct{ r CommitChecks }

func (c commitReader) CachedCommitChecks(q checktree.Query) (core.CommitChecks, bool) {
	return c.r.CachedCommitChecks(actions.CommitChecksQuery{Repo: q.Repo, SHA: q.SHA, Notes: q.Notes})
}

func (c commitReader) CommitChecks(ctx context.Context, q checktree.Query) (core.CommitChecks, error) {
	return c.r.CommitChecks(ctx, actions.CommitChecksQuery{Repo: q.Repo, SHA: q.SHA, Notes: q.Notes})
}
