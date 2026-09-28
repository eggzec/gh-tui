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

// CodeQuery selects a page of code search results.
type CodeQuery struct {
	// Text is the query in GitHub's code search syntax, qualifiers such as
	// repo:, language: and path: included.
	Text string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many files a page holds. Zero means the service's
	// page size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

// normalize returns q with its defaults set, as Query.normalize.
func (q CodeQuery) normalize(size int) CodeQuery {
	q.Text = normalizeText(q.Text)
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

// CachedCode returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedCode(q CodeQuery) (core.SearchPage[core.CodeHit], bool) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.CodeHit]{}, true
	}
	e, state := s.code.Get(codeKey(q))
	return e.Value, state != cache.Miss
}

// Code returns the page of files for q. A fresh cached page is returned
// without a request, and a query without text returns an empty page
// without one. Its total joins the counts that Search returns for the
// same text.
//
// GitHub allows 10 code searches a minute. Once they run out, the client
// fails a search at once with a *core.RateLimitError, which matches
// core.ErrRateLimited and says when code search resumes. A query GitHub
// can't run fails with a *core.InvalidQueryError.
func (s *Service) Code(ctx context.Context, q CodeQuery) (core.SearchPage[core.CodeHit], error) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.CodeHit]{}, nil
	}
	e, err := s.code.Fetch(ctx, codeKey(q), func(ctx context.Context, _ cache.Entry[core.SearchPage[core.CodeHit]], _ bool) (cache.Entry[core.SearchPage[core.CodeHit]], error) {
		p, err := s.api.SearchCode(ctx, q.Text, q.Cursor, q.PageSize)
		if err != nil {
			return cache.Entry[core.SearchPage[core.CodeHit]]{}, err
		}
		s.addCounts(q.Text, map[core.SearchKind]int{core.SearchCode: p.Total})
		return cache.Entry[core.SearchPage[core.CodeHit]]{Value: p, Tags: []string{allTag}}, nil
	})
	if err != nil {
		return core.SearchPage[core.CodeHit]{}, fmt.Errorf("search code %q: %w", q.Text, err)
	}
	return e.Value, nil
}

func codeKey(q CodeQuery) string {
	return "search/code?page_size=" + strconv.Itoa(q.PageSize) +
		"&cursor=" + url.QueryEscape(q.Cursor) + "&q=" + url.QueryEscape(strings.ToLower(q.Text))
}
