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

// TopicsQuery selects a page of topic search results.
type TopicsQuery struct {
	// Text is the query in GitHub's topic search syntax. It is passed on as
	// it is.
	Text string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many topics a page holds. Zero means the service's
	// page size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

// normalize returns q with its defaults set, as Query.normalize.
func (q TopicsQuery) normalize(size int) TopicsQuery {
	q.Text = normalizeText(q.Text)
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

func topicsKey(q TopicsQuery) string {
	return "search/topics?page_size=" + strconv.Itoa(q.PageSize) + "&cursor=" + url.QueryEscape(q.Cursor) +
		"&q=" + url.QueryEscape(strings.ToLower(q.Text))
}

// CachedTopics returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedTopics(q TopicsQuery) (core.SearchPage[core.SearchTopic], bool) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.SearchTopic]{}, true
	}
	e, state := s.topics.Get(topicsKey(q))
	return e.Value, state != cache.Miss
}

// Topics returns the page of topics for q. A fresh cached page is returned
// without a request, and a query without text returns an empty page without
// one.
//
// GitHub allows 30 searches a minute, shared with the other searches but
// code. Once they run out, the client fails a search at once with a
// *core.RateLimitError, which matches core.ErrRateLimited.
func (s *Service) Topics(ctx context.Context, q TopicsQuery) (core.SearchPage[core.SearchTopic], error) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.SearchTopic]{}, nil
	}
	e, err := s.topics.Fetch(ctx, topicsKey(q), func(ctx context.Context, _ cache.Entry[core.SearchPage[core.SearchTopic]], _ bool) (cache.Entry[core.SearchPage[core.SearchTopic]], error) {
		p, err := s.api.SearchTopics(ctx, q.Text, q.Cursor, q.PageSize)
		if err != nil {
			return cache.Entry[core.SearchPage[core.SearchTopic]]{}, err
		}
		return cache.Entry[core.SearchPage[core.SearchTopic]]{Value: p, Tags: []string{allTag}}, nil
	})
	if err != nil {
		return core.SearchPage[core.SearchTopic]{}, fmt.Errorf("search topics %q: %w", q.Text, err)
	}
	return e.Value, nil
}
