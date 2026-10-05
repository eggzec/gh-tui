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

// MaxAllRepos is the most repositories AllRepos reads of one owner, so
// that an organization with thousands costs a bounded number of requests.
const MaxAllRepos = 1000

// CachedAllRepos returns the repositories of q's owner from q.Cursor that
// the cached pages hold, up to limit as AllRepos counts them, without I/O.
// The page's Next is where the cached pages end, or empty if they reach
// the last page. It reports false if not even the first page is cached.
func (s *Service) CachedAllRepos(q ReposQuery, limit int) (core.Page[core.Repo], bool) {
	p, ok, _ := allRepos(q, limit, func(q ReposQuery) (core.Page[core.Repo], bool, error) {
		p, ok := s.CachedRepos(q)
		return p, ok, nil
	})
	return p, ok
}

// AllRepos reads the repositories of q's owner page by page from
// q.Cursor, the cached pages without a request, until the last page or
// until it holds limit repositories, so that a filter can run over them
// all. It reads whole pages, so it may hold a page's worth more. A limit
// that is not positive or above MaxAllRepos means MaxAllRepos. The page's
// Next is where reading stopped, or empty at the end; it is Stale, Offline
// or Limited if any page read was, and Again applies to every page.
func (s *Service) AllRepos(ctx context.Context, q ReposQuery, limit int) (core.Page[core.Repo], error) {
	p, _, err := allRepos(q, limit, func(q ReposQuery) (core.Page[core.Repo], bool, error) {
		p, err := s.Repos(ctx, q)
		return p, err == nil, err
	})
	return p, err
}

// allRepos gathers the pages read from q.Cursor until the last one or
// limit repositories. It stops at the first page read misses, and reports
// whether it read any.
func allRepos(q ReposQuery, limit int, read func(ReposQuery) (core.Page[core.Repo], bool, error)) (core.Page[core.Repo], bool, error) {
	if limit <= 0 || limit > MaxAllRepos {
		limit = MaxAllRepos
	}
	var all core.Page[core.Repo]
	all.Next = q.Cursor
	found := false
	for len(all.Items) < limit {
		q.Cursor = all.Next
		p, ok, err := read(q)
		if err != nil {
			return core.Page[core.Repo]{}, false, err
		}
		if !ok {
			break
		}
		found = true
		all.Items = append(all.Items, p.Items...)
		all.Next = p.Next
		all.Stale = all.Stale || p.Stale
		all.Offline = all.Offline || p.Offline
		all.Limited = all.Limited || p.Limited
		if p.Last() {
			break
		}
	}
	return all, found, nil
}
