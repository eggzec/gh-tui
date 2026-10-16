package pulls

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// timelineAPI is what the service reads the timeline of a pull request with.
type timelineAPI interface {
	// ListPullRequestTimeline returns a page of the timeline items of pull
	// request number, oldest first.
	ListPullRequestTimeline(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.TimelineItem], error)
}

// TimelineQuery selects the timeline of a pull request: its comments,
// reviews, commits and events, oldest first.
type TimelineQuery struct {
	Repo   core.RepoRef
	Number int
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many items the page holds at most. Zero means the
	// service's page size, and GitHub's maximum of 100 caps it.
	PageSize int
}

// key is the key of the page of q, as ListQuery.key.
func (q TimelineQuery) key(size int) string {
	return pageKey(detailKey(q.Repo, q.Number)+"/timeline", q.Cursor, pageSize(q.PageSize, size))
}

// CachedTimeline returns the page for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedTimeline(q TimelineQuery) (core.Page[core.TimelineItem], bool) {
	return cached(s.timelines, q.key(s.pageSize))
}

// Timeline returns the page for q, oldest first. A fresh cached page is
// returned without a request, and what an earlier session kept counts as
// cached. GraphQL gives the page no validators, so a stale one is read again
// in full. If GitHub can't be reached, a cached page is served with Offline
// set, and if it rate limits the read, with Limited set.
func (s *Service) Timeline(ctx context.Context, q TimelineQuery) (core.Page[core.TimelineItem], error) {
	key := q.key(s.pageSize)
	s.keptTimelines.Warm(s.timelines, key, true)
	p, err := fetch(ctx, s.timelines, s.keptTimelines, key, fallback.Page[core.TimelineItem], whole(tags(q.Repo, q.Number), func(ctx context.Context) (core.Page[core.TimelineItem], error) {
		return s.api.ListPullRequestTimeline(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize, s.pageSize))
	}))
	if err != nil {
		return core.Page[core.TimelineItem]{}, fmt.Errorf("list timeline of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}
