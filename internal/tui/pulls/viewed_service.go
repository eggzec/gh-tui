package pulls

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// viewedService is what the diff needs to show which files of a pull request
// the viewer has viewed, as GitHub keeps it, and to change that.
type viewedService interface {
	// CachedViewed returns the marks from memory, if they are there, without a
	// request.
	CachedViewed(q pulls.ViewedQuery) (core.PullViewed, bool)
	Viewed(ctx context.Context, q pulls.ViewedQuery) (core.PullViewed, error)
	// SetViewed shows the mark in the cache at once. The returned Op sends it.
	SetViewed(q pulls.ViewedQuery, path string, viewed bool) *optimistic.Op
}
