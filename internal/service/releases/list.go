package releases

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// ListQuery is a page of the releases of a repository.
type ListQuery struct {
	Repo core.RepoRef
	// Cursor is the Next of the previous page, or empty for the first.
	Cursor string
	// PageSize is how many releases a page holds; 0 is the service's size.
	PageSize int
	// Again makes a read revalidate a page that was only kept by an earlier
	// session, which is otherwise served stale. It is not part of the key.
	Again bool
}

// maxPageSize is the most GitHub returns in one page.
const maxPageSize = 100

func (q ListQuery) normalize(size int) ListQuery {
	if q.PageSize <= 0 {
		q.PageSize = size
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	return q
}

// SyncKey is the sync key under which a change to the releases of repo is
// reported.
func SyncKey(repo core.RepoRef) string { return "releases:" + repoID(repo) }

func repoID(r core.RepoRef) string { return strings.ToLower(r.String()) }

func listKey(q ListQuery) string {
	return fmt.Sprintf("list:%s:%d:%s", repoID(q.Repo), q.PageSize, q.Cursor)
}

// CachedList returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't cached.
func (s *Service) CachedList(q ListQuery) (core.Page[core.Release], bool) {
	e, st := s.lists.Get(listKey(q.normalize(s.pageSize)))
	if st == cache.Miss {
		return core.Page[core.Release]{}, false
	}
	p := e.Value
	p.Items = s.markAll(q.Repo, p.Items)
	return p, true
}

// List returns a page of the releases of q.Repo, newest first, with the
// repository's latest release marked. A fresh page comes from the cache;
// otherwise it is fetched, conditionally if a stale copy is cached. A page
// that only an earlier session kept is returned at once, with Stale set, to
// every read until one with q.Again set revalidates it. If GitHub can't be
// reached the stale page is served with Offline set, and if it rate limits
// the read, with Limited set.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Release], error) {
	q = q.normalize(s.pageSize)
	key := listKey(q)
	if e, ok := s.keptLists.Warm(s.lists, key, q.Again); ok {
		p := e.Value
		p.Stale = true
		p.Items = s.markAll(q.Repo, p.Items)
		return p, nil
	}
	e, err := fallback.Fetch(ctx, s.lists, s.keptLists, key, fallback.Page[core.Release],
		fallback.Keep(s.keptLists, key, recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Page[core.Release], github.Response, error) {
			p, resp, err := s.api.ListReleases(ctx, q.Repo, q.Cursor, q.PageSize, cond)
			if err == nil && q.Cursor == "" {
				// The first page is really fetched from GitHub: learn the
				// latest release now, so a kept or cached page costs no request.
				s.readLatest(ctx, q.Repo)
			}
			return p, resp, err
		}, func(core.Page[core.Release]) []string { return []string{"repo:" + repoID(q.Repo)} })))
	if err != nil {
		return core.Page[core.Release]{}, fmt.Errorf("list releases of %s: %w", q.Repo, err)
	}
	p := e.Value
	p.Items = s.markAll(q.Repo, p.Items)
	return p, nil
}

// Invalidate marks the cached list pages of repo stale. They are still served
// by CachedList, and the next List of each asks GitHub, conditionally, so a
// refresh reaches the server even while the pages are fresh, and costs no
// rate limit if nothing changed.
func (s *Service) Invalidate(repo core.RepoRef) {
	s.lists.InvalidateTag("repo:" + repoID(repo))
}

// AssetURL returns where GitHub serves file assetID of the releases of repo
// from. The address is signed and short-lived, so it is never cached.
func (s *Service) AssetURL(ctx context.Context, repo core.RepoRef, assetID int64) (string, error) {
	return s.api.ReleaseAssetURL(ctx, repo, assetID)
}

// readLatest learns the latest release of repo. It runs only when the first
// page of the list is fetched from GitHub, never for a kept or fresh page, so
// the mark can be one release behind after a background revalidation brought
// in a newer one, until the next read with Again. A failed read is recorded
// as "none" when nothing is known, so it is not retried before the next fetch;
// a known latest is kept.
func (s *Service) readLatest(ctx context.Context, repo core.RepoRef) {
	id := repoID(repo)
	r, ok, err := s.api.LatestRelease(ctx, repo)
	s.latestMu.Lock()
	defer s.latestMu.Unlock()
	switch {
	case err == nil && ok:
		s.latest[id] = r.ID
	case err == nil:
		s.latest[id] = 0
	default:
		if _, known := s.latest[id]; !known {
			s.latest[id] = 0
		}
	}
}

// mark returns r with Latest set if it is known to be the latest of repo.
func (s *Service) mark(repo core.RepoRef, r core.Release) core.Release {
	s.latestMu.Lock()
	id, ok := s.latest[repoID(repo)]
	s.latestMu.Unlock()
	r.Latest = ok && id != 0 && id == r.ID
	return r
}

// markAll marks a copy of items, which may be shared with the cache.
func (s *Service) markAll(repo core.RepoRef, items []core.Release) []core.Release {
	items = slices.Clone(items)
	for i := range items {
		items[i] = s.mark(repo, items[i])
	}
	return items
}

// Kept lists the pages and releases that the service keeps with validators,
// for a revalidator to check in the background.
func (s *Service) Kept() []revalidate.Entry {
	return slices.Concat(
		recheck.Entries(s.keptLists, kindList, s.ttl, s.listTarget),
		recheck.Entries(s.kept, kind, s.ttl, s.releaseTarget),
	)
}

func (s *Service) listTarget(key string) (recheck.Target, bool) {
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 || parts[0] != "list" {
		return recheck.Target{}, false
	}
	repo, err := core.ParseRepoRef(parts[1])
	size, serr := strconv.Atoi(parts[2])
	if err != nil || serr != nil {
		return recheck.Target{}, false
	}
	cursor := parts[3]
	return recheck.Target{Repo: repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Page[core.Release], github.Response, error) {
			return s.api.ListReleases(ctx, repo, cursor, size, cond)
		}, func(core.Page[core.Release]) []string { return []string{"repo:" + repoID(repo)} })
		_, res := recheck.Check(ctx, s.lists, s.keptLists, key, SyncKey(repo), load)
		return res
	}}, true
}

func (s *Service) releaseTarget(k string) (recheck.Target, bool) {
	rest, ok := strings.CutPrefix(k, "release:")
	if !ok {
		return recheck.Target{}, false
	}
	// The key is owner/name/id.
	repoKey, idKey, ok := strings.CutLast(rest, "/")
	if !ok {
		return recheck.Target{}, false
	}
	repo, err := core.ParseRepoRef(repoKey)
	id, nerr := strconv.ParseInt(idKey, 10, 64)
	if err != nil || nerr != nil {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Release, github.Response, error) {
			return s.api.GetRelease(ctx, repo, id, cond)
		}, func(core.Release) []string { return nil })
		_, res := recheck.Check(ctx, s.cache, s.kept, k, SyncKey(repo), load)
		return res
	}}, true
}
