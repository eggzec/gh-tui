package pulls

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/seen"
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
// of the pull request that the list last showed.
func (s *Service) Comments(ctx context.Context, q CommentsQuery) (core.Page[core.Comment], error) {
	if p, ok := s.currentComments(q); ok {
		return p, nil
	}
	// The page is at least as recent as what the list showed before the
	// read.
	m, _ := s.seen.Get(detailKey(q.Repo, q.Number))
	p, err := fetch(ctx, s.comments, q.key(), tags(q.Repo, q.Number), func(ctx context.Context) (seen.Stamped[core.Page[core.Comment]], error) {
		p, err := s.api.ListPullRequestComments(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize))
		return seen.Stamped[core.Page[core.Comment]]{Value: p, Version: m.updated}, err
	})
	if err != nil {
		return core.Page[core.Comment]{}, fmt.Errorf("list comments of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p.Value, nil
}

// CachedReviews returns the page for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedReviews(q ReviewsQuery) (core.Page[core.Review], bool) {
	return cached(s.reviews, q.key())
}

// Reviews returns the page for q, oldest first. A fresh cached page is
// returned without a request.
func (s *Service) Reviews(ctx context.Context, q ReviewsQuery) (core.Page[core.Review], error) {
	p, err := fetch(ctx, s.reviews, q.key(), tags(q.Repo, q.Number), func(ctx context.Context) (core.Page[core.Review], error) {
		return s.api.ListPullRequestReviews(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize))
	})
	if err != nil {
		return core.Page[core.Review]{}, fmt.Errorf("list reviews of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}
