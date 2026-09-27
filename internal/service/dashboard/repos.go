package dashboard

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// DefaultReposSize is the page size of a ReposQuery that sets none. Pages
// are as large as GitHub allows, since finding a repository by name needs
// them all.
const DefaultReposSize = maxPageSize

// MaxOwnerRepos is the most repositories AllRepos reads of one owner, so
// that an organization with thousands costs a bounded number of requests.
const MaxOwnerRepos = 1000

// ReposQuery selects a page of the repositories of one owner: the viewer's
// own, or those of an organization that the viewer can see. Either way they
// come most recently updated first.
type ReposQuery struct {
	// Viewer selects the repositories the viewer owns, and Owner is then
	// ignored. Otherwise Owner is the login of an organization.
	Viewer bool
	Owner  string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many repositories a page holds. Zero means
	// DefaultReposSize, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q ReposQuery) normalize() ReposQuery {
	if q.PageSize <= 0 {
		q.PageSize = DefaultReposSize
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	if q.Viewer {
		q.Owner = ""
	}
	return q
}

// key names the page. GitHub matches logins without regard to case, so
// keys do too, and "@me" can't be a login.
func (q ReposQuery) key() string {
	owner := "@me"
	if !q.Viewer {
		owner = strings.ToLower(q.Owner)
	}
	v := url.Values{"first": {strconv.Itoa(q.PageSize)}, "cursor": {q.Cursor}}
	return "ownerrepos:" + owner + "?" + v.Encode()
}

// CachedRepos returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedRepos(q ReposQuery) (core.Page[core.Repo], bool) {
	return s.repos.cached(q.normalize().key())
}

// FreshRepos reports whether the page for q is cached and fresh, so that
// reading it costs no request. It does no I/O.
func (s *Service) FreshRepos(q ReposQuery) bool {
	return s.repos.fresh(q.normalize().key())
}

// Repos returns the page for q. It is fresh for ReposTTL, and is served
// stale or offline as in Header. An organization that doesn't exist fails
// with an error matching core.ErrNotFound.
func (s *Service) Repos(ctx context.Context, q ReposQuery) (core.Page[core.Repo], error) {
	q = q.normalize()
	p, err := s.repos.get(ctx, q.key(), q.Again, func(ctx context.Context) (core.Page[core.Repo], error) {
		if q.Viewer {
			return s.api.ViewerOwnRepos(ctx, q.PageSize, q.Cursor)
		}
		return s.api.OrgRepos(ctx, q.Owner, q.PageSize, q.Cursor)
	})
	if err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("dashboard repos of %s: %w", ownerName(q), err)
	}
	return p, nil
}

// CachedAllRepos returns the repositories of the owner of q from the pages
// cached in a row from q.Cursor, without I/O, until it holds limit of
// them. The
// page's Next is where the cached pages end, or empty if they reach the
// last page. It reports false if not even the first page is cached.
func (s *Service) CachedAllRepos(q ReposQuery, limit int) (core.Page[core.Repo], bool) {
	p, ok, _ := s.allRepos(q, limit, func(q ReposQuery) (core.Page[core.Repo], bool, error) {
		p, ok := s.CachedRepos(q)
		return p, ok, nil
	})
	return p, ok
}

// AllRepos reads the repositories of the owner of q page by page from
// q.Cursor, the cached pages without a request, until the last page or
// until it holds limit repositories, so that the dashboard can find one by
// name. It reads whole pages, so it may hold a page's worth more. A limit
// that is not positive or above MaxOwnerRepos means MaxOwnerRepos. The
// page's Next is where reading stopped, or empty at the end; it is Stale
// or Offline if any page read was.
func (s *Service) AllRepos(ctx context.Context, q ReposQuery, limit int) (core.Page[core.Repo], error) {
	p, _, err := s.allRepos(q, limit, func(q ReposQuery) (core.Page[core.Repo], bool, error) {
		p, err := s.Repos(ctx, q)
		return p, err == nil, err
	})
	return p, err
}

// allRepos gathers the pages read from q.Cursor until the last one or
// limit repositories. It stops at the first page read misses, and reports
// whether it read any.
func (s *Service) allRepos(q ReposQuery, limit int, read func(ReposQuery) (core.Page[core.Repo], bool, error)) (core.Page[core.Repo], bool, error) {
	if limit <= 0 || limit > MaxOwnerRepos {
		limit = MaxOwnerRepos
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
		if p.Last() {
			break
		}
	}
	return all, found, nil
}

func ownerName(q ReposQuery) string {
	if q.Viewer {
		return "the viewer"
	}
	return q.Owner
}
