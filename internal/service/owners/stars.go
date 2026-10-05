package owners

import (
	"context"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// StarsQuery selects a page of the repositories a user starred.
type StarsQuery struct {
	Login string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many repositories a page holds. Zero means the
	// service's size for repositories, and sizes above GitHub's maximum of
	// 100 are clamped.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q StarsQuery) normalize(size int) StarsQuery {
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

func (q StarsQuery) key() string {
	v := url.Values{"first": {strconv.Itoa(q.PageSize)}, "cursor": {q.Cursor}}
	return "ownerstars:" + loginKey(q.Login) + "?" + v.Encode()
}

// CachedStars returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedStars(q StarsQuery) (core.Page[core.Repo], bool) {
	return s.repos.cached(q.normalize(s.sizes.Repos).key())
}

// FreshStars reports whether the page for q is cached and fresh, so that
// reading it costs no request. It does no I/O.
func (s *Service) FreshStars(q StarsQuery) bool {
	return s.repos.fresh(q.normalize(s.sizes.Repos).key())
}

// Stars returns the page for q, the latest starred first. It is a page of
// repositories like those of Repos: fresh for TTLs.Repos, and served
// stale, offline or limited, or failing for a user that isn't there, as
// in People.
func (s *Service) Stars(ctx context.Context, q StarsQuery) (core.Page[core.Repo], error) {
	q = q.normalize(s.sizes.Repos)
	// The client already names the request in its error.
	return read(ctx, s, &s.repos, q.Login, "stars", q.key(), q.Again, whole(func(ctx context.Context) (core.Page[core.Repo], error) {
		return s.api.UserStars(ctx, q.Login, q.PageSize, q.Cursor)
	}))
}
