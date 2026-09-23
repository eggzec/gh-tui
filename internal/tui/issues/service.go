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
	// Invalidate marks what is cached of repo stale, so that the reads
	// after it ask GitHub.
	Invalidate(repo core.RepoRef)
	// Close and Reopen show the change in the cache at once and return
	// the op that sends it.
	Close(repo core.RepoRef, number int) *optimistic.Op
	Reopen(repo core.RepoRef, number int) *optimistic.Op
	// Comment, AddLabels and RemoveLabel do the same for a new comment
	// and for labels.
	Comment(repo core.RepoRef, number int, body string) *optimistic.Op
	AddLabels(repo core.RepoRef, number int, names []string) *optimistic.Op
	RemoveLabel(repo core.RepoRef, number int, name string) *optimistic.Op
}
