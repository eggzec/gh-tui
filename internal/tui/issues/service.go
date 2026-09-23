package issues

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// Service is what the section needs from the issues service.
type Service interface {
	List(ctx context.Context, q issuesvc.ListQuery) (core.Page[core.Issue], error)
	Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error)
	CachedGet(repo core.RepoRef, number int) (core.Issue, bool)
	Comments(ctx context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error)
	// Close and Reopen show the change in the cache at once and return
	// the op that sends it.
	Close(repo core.RepoRef, number int) *optimistic.Op
	Reopen(repo core.RepoRef, number int) *optimistic.Op
}
