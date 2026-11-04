package issues

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// Dependencies returns the issues that block an issue (kind "blocked_by") or
// that it blocks (kind "blocking"). The read is not cached.
func (s *Service) Dependencies(ctx context.Context, repo core.RepoRef, number int, kind string) ([]core.Dependency, error) {
	return s.api.ListIssueDependencies(ctx, repo, number, kind)
}

// TimelineQuery selects a page of the timeline of an issue: its comments and
// events, oldest first.
type TimelineQuery struct {
	Repo   core.RepoRef
	Number int
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many items the page holds at most. Zero means the
	// service's page size, and GitHub's maximum of 100 caps it.
	PageSize int
}

func timelineKey(q TimelineQuery, size int) string {
	return fmt.Sprintf("timeline:%s#%d:%d:%s", repoID(q.Repo), q.Number, pageSize(q.PageSize, size), q.Cursor)
}

// CachedTimeline returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't cached.
func (s *Service) CachedTimeline(q TimelineQuery) (core.Page[core.TimelineItem], bool) {
	e, st := s.timelines.Get(timelineKey(q, s.pageSize))
	return e.Value, st != cache.Miss
}

// Timeline returns the page for q, oldest first. A fresh cached page is
// returned without a request, and what an earlier session kept counts as
// cached. GraphQL gives the page no validators, so a stale one is read again
// in full. If GitHub can't be reached, a cached page is served with Offline
// set, and if it rate limits the read, with Limited set.
func (s *Service) Timeline(ctx context.Context, q TimelineQuery) (core.Page[core.TimelineItem], error) {
	key := timelineKey(q, s.pageSize)
	s.keptTimelines.Warm(s.timelines, key, true)
	tags := []string{repoTag(q.Repo), issueKey(q.Repo, q.Number)}
	e, err := fallback.Fetch(ctx, s.timelines, s.keptTimelines, key, fallback.Page[core.TimelineItem], func(ctx context.Context, _ cache.Entry[core.Page[core.TimelineItem]], _ bool) (cache.Entry[core.Page[core.TimelineItem]], error) {
		p, err := s.api.ListIssueTimeline(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize, s.pageSize))
		if err != nil {
			return cache.Entry[core.Page[core.TimelineItem]]{}, err
		}
		e := cache.Entry[core.Page[core.TimelineItem]]{Value: p, Tags: tags}
		_ = s.keptTimelines.Save(key, e)
		return e, nil
	})
	if err != nil {
		return core.Page[core.TimelineItem]{}, fmt.Errorf("list timeline of issue %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value, nil
}
