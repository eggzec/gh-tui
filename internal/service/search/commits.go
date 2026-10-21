package search

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// CommitsQuery selects a page of commit search results.
type CommitsQuery struct {
	// Text is the query in GitHub's commit search syntax, qualifiers
	// included. It is passed on as it is.
	Text string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many commits a page holds. Zero means the service's
	// page size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

// normalize returns q with its defaults set, as Query.normalize.
func (q CommitsQuery) normalize(size int) CommitsQuery {
	q.Text = normalizeText(q.Text)
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

func commitsKey(q CommitsQuery) string {
	return "search/commits?page_size=" + strconv.Itoa(q.PageSize) + "&cursor=" + url.QueryEscape(q.Cursor) +
		"&q=" + url.QueryEscape(strings.ToLower(q.Text))
}

// CachedCommits returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedCommits(q CommitsQuery) (core.SearchPage[core.SearchCommit], bool) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.SearchCommit]{}, true
	}
	e, state := s.commits.Get(commitsKey(q))
	return e.Value, state != cache.Miss
}

// Commits returns the page of commits for q. A fresh cached page is returned
// without a request, and a query without text returns an empty page without
// one.
//
// GitHub allows 30 searches a minute, shared with the other searches but
// code. Once they run out, the client fails a search at once with a
// *core.RateLimitError, which matches core.ErrRateLimited.
func (s *Service) Commits(ctx context.Context, q CommitsQuery) (core.SearchPage[core.SearchCommit], error) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.SearchCommit]{}, nil
	}
	e, err := s.commits.Fetch(ctx, commitsKey(q), func(ctx context.Context, _ cache.Entry[core.SearchPage[core.SearchCommit]], _ bool) (cache.Entry[core.SearchPage[core.SearchCommit]], error) {
		p, err := s.api.SearchCommits(ctx, q.Text, q.Cursor, q.PageSize)
		if err != nil {
			return cache.Entry[core.SearchPage[core.SearchCommit]]{}, err
		}
		return cache.Entry[core.SearchPage[core.SearchCommit]]{Value: p, Tags: []string{allTag}}, nil
	})
	if err != nil {
		return core.SearchPage[core.SearchCommit]{}, fmt.Errorf("search commits %q: %w", q.Text, err)
	}
	return e.Value, nil
}
