package pulls

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
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
	return cached(s.comments, q.key())
}

// Comments returns the page for q, oldest first. A fresh cached page is
// returned without a request.
func (s *Service) Comments(ctx context.Context, q CommentsQuery) (core.Page[core.Comment], error) {
	p, err := fetch(ctx, s.comments, q.key(), q.Repo, func(ctx context.Context) (core.Page[core.Comment], error) {
		return s.api.ListPullRequestComments(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize))
	})
	if err != nil {
		return core.Page[core.Comment]{}, fmt.Errorf("list comments of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}

// CachedReviews returns the page for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedReviews(q ReviewsQuery) (core.Page[core.Review], bool) {
	return cached(s.reviews, q.key())
}

// Reviews returns the page for q, oldest first. A fresh cached page is
// returned without a request.
func (s *Service) Reviews(ctx context.Context, q ReviewsQuery) (core.Page[core.Review], error) {
	p, err := fetch(ctx, s.reviews, q.key(), q.Repo, func(ctx context.Context) (core.Page[core.Review], error) {
		return s.api.ListPullRequestReviews(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize))
	})
	if err != nil {
		return core.Page[core.Review]{}, fmt.Errorf("list reviews of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}
