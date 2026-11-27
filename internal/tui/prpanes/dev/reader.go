package dev

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// PreviewService is what the previews of the rows read. *pulls.Service
// satisfies it.
type PreviewService interface {
	CachedIssuePreview(q pulls.PreviewQuery) (core.IssuePreview, bool)
	IssuePreview(ctx context.Context, q pulls.PreviewQuery) (core.IssuePreview, error)
	CachedPullPreview(q pulls.PreviewQuery) (core.PullPreview, bool)
	PullPreview(ctx context.Context, q pulls.PreviewQuery) (core.PullPreview, error)
	// CachedComments and Comments page the comments of an issue as well as
	// of a pull request, since both are comments of an issue to GitHub.
	CachedComments(q pulls.CommentsQuery) (core.Page[core.Comment], bool)
	Comments(ctx context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error)
}

var _ PreviewService = (*pulls.Service)(nil)

// ServiceReader adapts a PreviewService to the Reader of the previews.
func ServiceReader(s PreviewService) Reader { return serviceReader{s} }

type serviceReader struct{ s PreviewService }

func previewQuery(t core.Target) pulls.PreviewQuery {
	return pulls.PreviewQuery{Repo: t.Repo, Number: t.Number}
}

func commentsQuery(t core.Target, cursor string) pulls.CommentsQuery {
	return pulls.CommentsQuery{Repo: t.Repo, Number: t.Number, Cursor: cursor}
}

func (r serviceReader) CachedComments(t core.Target, cursor string) (core.Page[core.Comment], bool) {
	return r.s.CachedComments(commentsQuery(t, cursor))
}

func (r serviceReader) Comments(ctx context.Context, t core.Target, cursor string) (core.Page[core.Comment], error) {
	return r.s.Comments(ctx, commentsQuery(t, cursor))
}

func (r serviceReader) CachedIssue(t core.Target) (core.IssuePreview, bool) {
	return r.s.CachedIssuePreview(previewQuery(t))
}

func (r serviceReader) Issue(ctx context.Context, t core.Target) (core.IssuePreview, error) {
	return r.s.IssuePreview(ctx, previewQuery(t))
}

func (r serviceReader) CachedPull(t core.Target) (core.PullPreview, bool) {
	return r.s.CachedPullPreview(previewQuery(t))
}

func (r serviceReader) Pull(ctx context.Context, t core.Target) (core.PullPreview, error) {
	return r.s.PullPreview(ctx, previewQuery(t))
}
