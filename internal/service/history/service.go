// Package history reads the history of GitHub repositories: their
// branches, the commits of a branch, what one commit changed, and how far
// apart two refs are.
//
// What a ref names moves, so the branches and the first page of the
// commits of a ref are revalidated with their ETags, which costs no rate
// limit when nothing moved, and are kept on a shelf of the account's own
// for the revalidator to recheck. Everything else is named by a commit SHA
// and never changes: the later pages of a history, which the first page
// pins to the commit the ref pointed at, and the changes of a commit. Those
// are read from memory, then from the object store, and only then from
// GitHub, and are kept for good.
//
// Pinning keeps paging correct when a branch moves between two pages: page
// two continues the history that page one showed, rather than shifting by
// the commits pushed meanwhile. While the ref doesn't move, its first page
// revalidates with a 304 and every later page is a hit. Once it moves, the
// first page brings a new head, whose later pages are read anew. They
// aren't derived from the old ones: git log orders commits by date across
// merges, so a new head's history needn't continue the old one's pages.
package history

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// API is the part of the GitHub client that the service uses.
type API interface {
	ListBranches(ctx context.Context, repo core.RepoRef, cursor string, perPage int, cond github.Conditional) (core.Page[core.Branch], github.Response, error)
	ListCommits(ctx context.Context, repo core.RepoRef, ref, cursor string, perPage int, cond github.Conditional) (core.Page[core.Commit], github.Response, error)
	GetCommit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitDetail, error)
	ListCommitFiles(ctx context.Context, repo core.RepoRef, sha, cursor string) (core.Page[core.CommitFile], error)
	Compare(ctx context.Context, repo core.RepoRef, base, head string, cond github.Conditional) (core.Compare, github.Response, error)
}

// Service reads branches, commits and comparisons. It is safe for
// concurrent use.
type Service struct {
	api API
	// ttl is how long branches and the first pages of refs stay fresh.
	ttl      time.Duration
	branches *cache.Cache[core.Page[core.Branch]]
	// refPages holds the first page of the commits of a ref, which moves;
	// pages holds the pages named by a SHA, which don't.
	refPages *cache.Cache[core.Page[core.Commit]]
	pages    *cache.Cache[core.Page[core.Commit]]
	details  *cache.Cache[core.CommitDetail]
	files    *cache.Cache[core.Page[core.CommitFile]]
	compares *cache.Cache[core.Compare]

	// The account's shelf keeps what is revalidated; the object shelves
	// keep what a SHA names.
	keptBranches *cache.Shelf[core.Page[core.Branch]]
	keptRefPages *cache.Shelf[core.Page[core.Commit]]
	keptPages    *cache.Shelf[core.Page[core.Commit]]
	keptDetails  *cache.Shelf[core.CommitDetail]
	keptFiles    *cache.Shelf[core.Page[core.CommitFile]]
	// commitPageSize is the size of a page of commits whose query sets
	// none.
	commitPageSize int
}

// The kinds of entries the service keeps, and the version of their values.
// Bump schema when the core types they hold change shape.
const (
	kindBranches = "branchlist"
	kindRefPages = "commitlist"
	kindPages    = "commitpage"
	kindDetails  = "commit"
	kindFiles    = "commitfiles"
	schema       = 1
)

// New returns a service that calls api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	ttl := cmp.Or(o.ttl, d.Cache.TTL.History)
	capacity := cache.WithCapacity(cmp.Or(o.capacity, d.Cache.Memory.Entries))
	diffs := cmp.Or(o.diffMemory, int64(d.Cache.Memory.Diffs))
	// What a SHA names never changes, so it never goes stale.
	return &Service{
		api:          api,
		ttl:          ttl,
		branches:     cache.New[core.Page[core.Branch]](cache.WithTTL(ttl), capacity),
		refPages:     cache.New[core.Page[core.Commit]](cache.WithTTL(ttl), capacity),
		pages:        cache.New[core.Page[core.Commit]](capacity),
		details:      cache.New[core.CommitDetail](capacity, cache.WithMaxSize(diffs, detailSize)),
		files:        cache.New[core.Page[core.CommitFile]](capacity, cache.WithMaxSize(diffs, filesSize)),
		compares:     cache.New[core.Compare](cache.WithTTL(cmp.Or(o.compareTTL, d.Cache.TTL.Compare)), capacity),
		keptBranches: cache.NewShelf[core.Page[core.Branch]](o.store, kindBranches, schema),
		keptRefPages: cache.NewShelf[core.Page[core.Commit]](o.store, kindRefPages, schema),
		keptPages:    cache.NewShelf[core.Page[core.Commit]](o.objects, kindPages, schema),
		keptDetails:  cache.NewShelf[core.CommitDetail](o.objects, kindDetails, schema),
		keptFiles:    cache.NewShelf[core.Page[core.CommitFile]](o.objects, kindFiles, schema),

		commitPageSize: cmp.Or(o.commitPageSize, d.PageSize.Commits),
	}
}

// Invalidate marks the branches, the first pages of commits and the
// comparisons of repo stale, so the next read of each asks GitHub, with a
// conditional request. What a SHA names can't change and is kept.
func (s *Service) Invalidate(repo core.RepoRef) {
	tag := repoTag(repo)
	s.branches.InvalidateTag(tag)
	s.refPages.InvalidateTag(tag)
	s.compares.InvalidateTag(tag)
}

// SyncKey names changes to the branches of repo, or to where they point,
// in sync events.
func SyncKey(repo core.RepoRef) string {
	return "history:" + repoKey(repo)
}

// fetch reads key from c, or loads it with load when it is missing or
// stale, falling back on the stale entry as fallback.Fetch does, marked by
// marks. A stale entry's validators make the request conditional. What
// GitHub sends is kept on shelf too.
func fetch[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key string, tags []string, marks fallback.Marks[V],
	load func(ctx context.Context, cond github.Conditional) (V, github.Response, error),
) (V, error) {
	e, err := fallback.Fetch(ctx, c, shelf, key, marks, fallback.Keep(shelf, key, recheck.Load(load, func(V) []string { return tags })))
	return e.Value, err
}

// object reads what key names for good from c, then from shelf, and then
// with load, and keeps what load returns in both.
func object[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key string, load func(ctx context.Context) (V, error)) (V, error) {
	e, err := c.Fetch(ctx, key, func(ctx context.Context, _ cache.Entry[V], _ bool) (cache.Entry[V], error) {
		if kept, ok := shelf.Load(key); ok {
			return cache.Entry[V]{Value: kept.Value}, nil
		}
		v, err := load(ctx)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		e := cache.Entry[V]{Value: v}
		_ = shelf.Save(key, e)
		return e, nil
	})
	return e.Value, err
}

// pageSize returns n, or def for 0, at most the 100 that GitHub lists.
func pageSize(n, def int) int {
	if n <= 0 {
		n = def
	}
	return min(n, maxPageSize)
}

// maxPageSize is the most items GitHub returns in a page.
const maxPageSize = 100

func repoTag(repo core.RepoRef) string {
	return "repo:" + repoKey(repo)
}

// isSHA reports whether ref is a full SHA-1 or SHA-256 object name. A
// branch could have such a name too, but git warns against it.
func isSHA(ref string) bool {
	if len(ref) != 40 && len(ref) != 64 {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}

// repoKey formats repo for a cache key: GitHub matches names without
// regard to case.
func repoKey(repo core.RepoRef) string {
	return strings.ToLower(repo.String())
}

// errNotSHA refuses to read a commit by anything but its full SHA, which is
// what makes it immutable.
var errNotSHA = errors.New("not a full commit SHA")
