package history

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
)

// Service is what the modal needs from the history service.
type Service interface {
	Branches(ctx context.Context, q historysvc.BranchesQuery) (core.Page[core.Branch], error)
	Commits(ctx context.Context, q historysvc.CommitsQuery) (core.Page[core.Commit], error)
	// CachedCommit returns the detail of a commit from memory, without a
	// request.
	CachedCommit(repo core.RepoRef, sha string) (core.CommitDetail, bool)
	Commit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitDetail, error)
	CommitFiles(ctx context.Context, q historysvc.CommitFilesQuery) (core.Page[core.CommitFile], error)
	// CachedCompare returns a comparison from memory, without a request.
	CachedCompare(repo core.RepoRef, base, head string) (core.Compare, bool)
	Compare(ctx context.Context, repo core.RepoRef, base, head string) (core.Compare, error)
}
