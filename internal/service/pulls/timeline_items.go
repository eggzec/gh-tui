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
	// PageSize is how many items one request reads at most. Zero means the
	// service's page size, and GitHub's maximum of 100 caps it. It only sets
	// how the timeline is read, never what it holds, but keys the cache.
	PageSize int
}

// key is the key of the timeline of q.
func (q TimelineQuery) key(size int) string {
	return pageKey(detailKey(q.Repo, q.Number)+"/timeline", "", pageSize(q.PageSize, size))
}

// CachedTimeline returns the timeline for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedTimeline(q TimelineQuery) (core.Page[core.TimelineItem], bool) {
	return cached(s.timelines, q.key(s.pageSize))
}

// Timeline returns every item of the timeline for q, oldest first, in one
// page that has no Next. A fresh cached timeline is returned without a
// request, and what an earlier session kept counts as cached. GraphQL gives
// the pages no validators, so a stale timeline is read again in full. If
// GitHub can't be reached, a cached timeline is served with Offline set, and
// if it rate limits the read, with Limited set.
func (s *Service) Timeline(ctx context.Context, q TimelineQuery) (core.Page[core.TimelineItem], error) {
	key := q.key(s.pageSize)
	s.keptTimelines.Warm(s.timelines, key, true)
	p, err := fetch(ctx, s.timelines, s.keptTimelines, key, fallback.Page[core.TimelineItem], whole(tags(q.Repo, q.Number), func(ctx context.Context) (core.Page[core.TimelineItem], error) {
		return s.readTimeline(ctx, q)
	}))
	if err != nil {
		return core.Page[core.TimelineItem]{}, fmt.Errorf("list timeline of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}

// readTimeline reads the pages of the timeline for q one after the other. A
// page that fails fails the read, since a timeline with a hole misleads.
func (s *Service) readTimeline(ctx context.Context, q TimelineQuery) (core.Page[core.TimelineItem], error) {
	var all []core.TimelineItem
	cursor := ""
	for {
		p, err := s.api.ListPullRequestTimeline(ctx, q.Repo, q.Number, cursor, pageSize(q.PageSize, s.pageSize))
		if err != nil {
			return core.Page[core.TimelineItem]{}, err
		}
		all = append(all, p.Items...)
		// A cursor that doesn't move would loop for ever.
		if p.Next == "" || p.Next == cursor {
			return core.Page[core.TimelineItem]{Items: all}, nil
		}
		cursor = p.Next
	}
}
