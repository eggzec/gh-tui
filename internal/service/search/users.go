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

// UsersQuery selects a page of user or organization search results.
type UsersQuery struct {
	// Text is the query in GitHub's user search syntax. A type: qualifier
	// the user typed is kept as it is.
	Text string
	// Kind is core.SearchUsers or core.SearchOrgs. Any other kind means
	// users.
	Kind core.SearchKind
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many accounts a page holds. Zero means the service's
	// page size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

// normalize returns q with its defaults set, as Query.normalize.
func (q UsersQuery) normalize(size int) UsersQuery {
	q.Text = normalizeText(q.Text)
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

// request returns the query GitHub is asked: Text with the type: qualifier
// of Kind, unless Text has one.
func (q UsersQuery) request() string {
	for f := range strings.FieldsSeq(q.Text) {
		if strings.HasPrefix(strings.ToLower(f), "type:") {
			return q.Text
		}
	}
	if q.Kind == core.SearchOrgs {
		return q.Text + " type:org"
	}
	return q.Text + " type:user"
}

// usersKey keys a page by the query GitHub is asked, so that users and
// organizations of one text are cached apart.
func usersKey(q UsersQuery) string {
	return "search/users?page_size=" + strconv.Itoa(q.PageSize) + "&cursor=" + url.QueryEscape(q.Cursor) +
		"&q=" + url.QueryEscape(strings.ToLower(q.request()))
}

// CachedUsers returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedUsers(q UsersQuery) (core.SearchPage[core.SearchUser], bool) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.SearchUser]{}, true
	}
	e, state := s.users.Get(usersKey(q))
	return e.Value, state != cache.Miss
}

// Users returns the page of users, or of organizations, for q. A fresh
// cached page is returned without a request, and a query without text
// returns an empty page without one.
//
// GitHub allows 30 searches a minute, shared with the other searches but
// code. Once they run out, the client fails a search at once with a
// *core.RateLimitError, which matches core.ErrRateLimited.
func (s *Service) Users(ctx context.Context, q UsersQuery) (core.SearchPage[core.SearchUser], error) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return core.SearchPage[core.SearchUser]{}, nil
	}
	e, err := s.users.Fetch(ctx, usersKey(q), func(ctx context.Context, _ cache.Entry[core.SearchPage[core.SearchUser]], _ bool) (cache.Entry[core.SearchPage[core.SearchUser]], error) {
		p, err := s.api.SearchUsers(ctx, q.request(), q.Cursor, q.PageSize)
		if err != nil {
			return cache.Entry[core.SearchPage[core.SearchUser]]{}, err
		}
		return cache.Entry[core.SearchPage[core.SearchUser]]{Value: p, Tags: []string{allTag}}, nil
	})
	if err != nil {
		return core.SearchPage[core.SearchUser]{}, fmt.Errorf("search users %q: %w", q.Text, err)
	}
	return e.Value, nil
}
