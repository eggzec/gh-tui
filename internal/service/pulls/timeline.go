package pulls

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// A pull request's comments and reviews are read like a pager: a page at a
// time, oldest first, each page cached on its own under its cursor and size.

// CommentsQuery selects a page of the comments on a pull request.
type CommentsQuery struct {
	Repo   core.RepoRef
	Number int
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many comments the page holds at most. Zero means 30,
	// and GitHub's maximum of 100 caps it.
	PageSize int
}

func (q CommentsQuery) key() string {
	return pageKey(detailKey(q.Repo, q.Number)+"/comments", q.Cursor, q.PageSize)
}

// ReviewsQuery selects a page of the reviews of a pull request.
type ReviewsQuery struct {
	Repo   core.RepoRef
	Number int
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many reviews the page holds at most. Zero means 30,
	// and GitHub's maximum of 100 caps it.
	PageSize int
}

func (q ReviewsQuery) key() string {
	return pageKey(detailKey(q.Repo, q.Number)+"/reviews", q.Cursor, q.PageSize)
}

// pageKey is the key of the page of prefix at cursor with size items.
func pageKey(prefix, cursor string, size int) string {
	v := url.Values{"cursor": {cursor}, "first": {strconv.Itoa(pageSize(size))}}
	return prefix + "?" + v.Encode()
}

// CachedComments returns the page for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedComments(q CommentsQuery) (core.Page[core.Comment], bool) {
	p, ok := cached(s.comments, q.key())
	return p.Value, ok
}

// Comments returns the page for q, oldest first. A cached page is returned
// without a request while it is fresh, or while it was read at the version
// of the pull request that the list last showed. Otherwise it is read with
// REST, conditionally if a stale copy is cached, so that a page that didn't
// change costs no rate limit. What an earlier session kept counts as
// cached, as for Get.
func (s *Service) Comments(ctx context.Context, q CommentsQuery) (core.Page[core.Comment], error) {
	key := q.key()
	s.keptComments.Warm(s.comments, key)
	if p, ok := s.currentComments(q); ok {
		return p, nil
	}
	// The page is at least as recent as what the list showed before the
	// read.
	m, _ := s.seen.Get(detailKey(q.Repo, q.Number))
	e, err := s.comments.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[stampedComments], ok bool) (cache.Entry[stampedComments], error) {
		e, err := s.loadComments(q, m.updated)(ctx, prev, ok)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.Value, prev.FetchedAt = offlineComments(prev.Value), offlineAt
			return prev, nil
		case errors.Is(err, cache.ErrNotModified):
			// The cached page is current as of now, so it takes the
			// newer version. The kept page stays as GitHub sent it, marked
			// fetched now if it is the one GitHub confirmed.
			restamp(s.keptComments, key, prev)
			prev.Value.Version, prev.FetchedAt = m.updated, time.Time{}
			return prev, nil
		case err != nil:
			if github.Refused(err) {
				s.keptComments.Delete(key)
			}
			return prev, err
		}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = s.keptComments.Save(key, e)
		return e, nil
	})
	if err != nil {
		return core.Page[core.Comment]{}, fmt.Errorf("list comments of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value.Value, nil
}

// loadComments reads the page for q with its validators, if it has a
// cached one, and stamps it with version. It reports cache.ErrNotModified
// when the cached page is current.
func (s *Service) loadComments(q CommentsQuery, version time.Time) cache.FetchFunc[stampedComments] {
	return recheck.Load(func(ctx context.Context, cond github.Conditional) (stampedComments, github.Response, error) {
		// A pull request's comments are those of its issue.
		p, res, err := s.api.ListIssueComments(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize), cond)
		return stampedComments{Value: p, Version: version}, res, err
	}, func(stampedComments) []string { return tags(q.Repo, q.Number) })
}

// offlineComments marks a page of comments served offline.
func offlineComments(p stampedComments) stampedComments {
	p.Value.Offline = true
	return p
}

// CachedReviews returns the page for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedReviews(q ReviewsQuery) (core.Page[core.Review], bool) {
	return cached(s.reviews, q.key())
}

// Reviews returns the page for q, oldest first. A fresh cached page is
// returned without a request.
func (s *Service) Reviews(ctx context.Context, q ReviewsQuery) (core.Page[core.Review], error) {
	p, err := fetch(ctx, s.reviews, nil, q.key(), offlinePage[core.Review], whole(tags(q.Repo, q.Number), func(ctx context.Context) (core.Page[core.Review], error) {
		return s.api.ListPullRequestReviews(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize))
	}))
	if err != nil {
		return core.Page[core.Review]{}, fmt.Errorf("list reviews of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}
