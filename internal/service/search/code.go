package search

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	// PageSize is how many files a page holds. Zero means DefaultPageSize,
	// and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

func (q CodeQuery) normalize() CodeQuery {
	q.Text = normalizeText(q.Text)
	q.PageSize = pageSize(q.PageSize)
	return q
}

// CachedCode returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedCode(q CodeQuery) (core.SearchPage[core.CodeHit], bool) {
	q = q.normalize()
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
// GitHub allows 10 code searches a minute. Once it refuses one, Code fails
// at once with a *core.RateLimitError, which matches core.ErrRateLimited
// and says when code search resumes, until then. A query GitHub can't run
// fails with a *core.InvalidQueryError.
func (s *Service) Code(ctx context.Context, q CodeQuery) (core.SearchPage[core.CodeHit], error) {
	q = q.normalize()
	if q.Text == "" {
		return core.SearchPage[core.CodeHit]{}, nil
	}
	e, err := s.code.Fetch(ctx, codeKey(q), func(ctx context.Context, _ cache.Entry[core.SearchPage[core.CodeHit]], _ bool) (cache.Entry[core.SearchPage[core.CodeHit]], error) {
		if reset, limited := s.CodeLimited(); limited {
			return cache.Entry[core.SearchPage[core.CodeHit]]{}, &core.RateLimitError{Reset: reset}
		}
		p, err := s.api.SearchCode(ctx, q.Text, q.Cursor, q.PageSize)
		if err != nil {
			if rl, ok := errors.AsType[*core.RateLimitError](err); ok {
				s.limitCode(rl.Reset)
			}
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

// CodeLimited reports whether code search is out of requests, as GitHub
// said when it last refused one, and when it resumes.
func (s *Service) CodeLimited() (reset time.Time, limited bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !time.Now().Before(s.codeReset) {
		return time.Time{}, false
	}
	return s.codeReset, true
}

func (s *Service) limitCode(reset time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codeReset = reset
}

func codeKey(q CodeQuery) string {
	return "search/code?page_size=" + strconv.Itoa(q.PageSize) +
		"&cursor=" + url.QueryEscape(q.Cursor) + "&q=" + url.QueryEscape(strings.ToLower(q.Text))
}
