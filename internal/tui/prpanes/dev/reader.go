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
}

var _ PreviewService = (*pulls.Service)(nil)

// ServiceReader adapts a PreviewService to the Reader of the previews.
func ServiceReader(s PreviewService) Reader { return serviceReader{s} }

type serviceReader struct{ s PreviewService }

func previewQuery(t core.Target) pulls.PreviewQuery {
	return pulls.PreviewQuery{Repo: t.Repo, Number: t.Number}
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
