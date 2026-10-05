package owners

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// maxPageSize is the largest page GitHub returns.
const maxPageSize = 100

// ReposQuery selects a page of the repositories an account owns that the
// viewer can see.
type ReposQuery struct {
	// Owner is the login of the user or the organization, and Kind which
	// of the two it is, as its header says.
	Owner string
	Kind  core.OwnerKind
	// Order sorts a user's repositories. An organization's come most
	// recently updated first, whatever it says.
	Order core.RepoOrder
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many repositories a page holds. Zero means the
	// service's size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q ReposQuery) normalize(size int) ReposQuery {
	if q.PageSize <= 0 {
		q.PageSize = size
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	if q.Kind == core.OwnerOrg {
		q.Order = core.RepoOrder{}
	}
	return q
}

// key names the page. The order is in it for a user only, whose pages it
// sorts.
func (q ReposQuery) key() string {
	v := url.Values{"first": {strconv.Itoa(q.PageSize)}, "cursor": {q.Cursor}}
	if q.Kind == core.OwnerOrg {
		v.Set("org", "1")
	} else {
		v.Set("order", strconv.Itoa(int(q.Order.Field)))
		v.Set("asc", strconv.FormatBool(q.Order.Ascending))
	}
	return "ownerrepos:" + loginKey(q.Owner) + "?" + v.Encode()
}

// scope is the kind of read that a missing account fails, for read: a
// user's repositories or an organization's.
func (q ReposQuery) scope() string {
	if q.Kind == core.OwnerOrg {
		return "orgrepos"
	}
	return "userrepos"
}

// CachedRepos returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedRepos(q ReposQuery) (core.Page[core.Repo], bool) {
	return s.repos.cached(q.normalize(s.sizes.Repos).key())
}

// FreshRepos reports whether the page for q is cached and fresh, so that
// reading it costs no request. It does no I/O.
func (s *Service) FreshRepos(q ReposQuery) bool {
	return s.repos.fresh(q.normalize(s.sizes.Repos).key())
}

// Repos returns the page for q. It is fresh for TTLs.Repos, and is served
// stale, offline or limited, or fails for an account that isn't there, as
// in Header.
func (s *Service) Repos(ctx context.Context, q ReposQuery) (core.Page[core.Repo], error) {
	q = q.normalize(s.sizes.Repos)
	p, err := read(ctx, s, &s.repos, q.Owner, q.scope(), q.key(), q.Again, whole(func(ctx context.Context) (core.Page[core.Repo], error) {
		if q.Kind == core.OwnerOrg {
			return s.api.OrgRepos(ctx, q.Owner, q.PageSize, q.Cursor)
		}
		return s.api.UserRepos(ctx, q.Owner, q.Order, q.PageSize, q.Cursor)
	}))
	if err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("repos of %s: %w", q.Owner, err)
	}
	return p, nil
}
