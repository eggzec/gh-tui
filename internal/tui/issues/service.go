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
	// FreshList reports whether List returns the page of q without a
	// request. It may do I/O.
	FreshList(q issuesvc.ListQuery) bool
	Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error)
	CachedGet(repo core.RepoRef, number int) (core.Issue, bool)
	CachedComments(q issuesvc.CommentsQuery) (core.Page[core.Comment], bool)
	Comments(ctx context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error)
	// CachedTimeline and Timeline read a page of the timeline of an issue:
	// its comments and events.
	CachedTimeline(q issuesvc.TimelineQuery) (core.Page[core.TimelineItem], bool)
	Timeline(ctx context.Context, q issuesvc.TimelineQuery) (core.Page[core.TimelineItem], error)
	// CachedPinned and Pinned read the pinned issues of a repository, in pin
	// order. They are apart from the pages of List.
	CachedPinned(repo core.RepoRef) ([]core.Issue, bool)
	Pinned(ctx context.Context, repo core.RepoRef) ([]core.Issue, error)
	// CurrentGet and CurrentComments report whether Get and Comments
	// would answer without a request. They do no I/O.
	CurrentGet(repo core.RepoRef, number int) bool
	CurrentComments(q issuesvc.CommentsQuery) bool
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
	// DeleteComment removes the comment with the id from the cache at once
	// and returns the op that deletes it.
	DeleteComment(repo core.RepoRef, number int, id string) *optimistic.Op
}
