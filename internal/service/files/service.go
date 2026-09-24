// Package files browses the files of GitHub repositories: one directory of a
// tree at a time, every path of a tree at once for search, and the content of
// a file. Git objects never change once they exist, so whatever is read by
// SHA stays cached for good; only what a ref such as a branch points at is
// revalidated.
package files

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// API is the part of the GitHub client that the service uses.
type API interface {
	GetTree(ctx context.Context, repo core.RepoRef, tree string, cond github.Conditional) (core.Tree, github.Response, error)
	GetTreeRecursive(ctx context.Context, repo core.RepoRef, tree string, cond github.Conditional) (core.Tree, github.Response, error)
	GetBlob(ctx context.Context, repo core.RepoRef, sha string, limit int64) (core.Blob, error)
}

// forever is the TTL of entries read by SHA, which can't change.
const forever = time.Duration(math.MaxInt64)

// Service reads trees and blobs. It is safe for concurrent use.
type Service struct {
	api     API
	maxBlob int64
	// refs holds trees read by a ref, which may move, so they go stale and
	// are revalidated with their ETag.
	refs *cache.Cache[core.Tree]
	// objects holds trees read by SHA, including the ones read by a ref,
	// which are also stored under the SHA they resolved to.
	objects *cache.Cache[core.Tree]
	blobs   *cache.Cache[core.Blob]
}

// New returns a service that calls api.
func New(api API, opts ...Option) *Service {
	o := options{blobCapacity: DefaultBlobCapacity, maxBlob: DefaultMaxBlobSize}
	for _, opt := range opts {
		opt(&o)
	}
	immutable := slices.Concat(o.cache, []cache.Option{cache.WithTTL(forever)})
	return &Service{
		api:     api,
		maxBlob: o.maxBlob,
		refs:    cache.New[core.Tree](o.cache...),
		objects: cache.New[core.Tree](immutable...),
		blobs:   cache.New[core.Blob](cache.WithCapacity(o.blobCapacity), cache.WithTTL(forever)),
	}
}

// fetch reads key from c, or loads it with load when it is missing or
// stale. A stale entry's validators make the request conditional.
func fetch[V any](ctx context.Context, c *cache.Cache[V], key string, tags []string,
	load func(ctx context.Context, cond github.Conditional) (V, github.Response, error),
) (V, error) {
	e, err := c.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		v, res, err := load(ctx, cond)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		if res.NotModified {
			return cache.Entry[V]{}, cache.ErrNotModified
		}
		return cache.Entry[V]{Value: v, ETag: res.ETag, LastModified: res.LastModified, Tags: tags}, nil
	})
	return e.Value, err
}

// Invalidate marks the trees that refs of repo point at stale, so the next
// read asks GitHub whether the refs moved. It costs no rate limit if they
// didn't. Trees and blobs read by SHA can't change and are kept.
func (s *Service) Invalidate(repo core.RepoRef) {
	s.refs.InvalidateTag(repoTag(repo))
}

// Cache keys and tags.

func treeKey(repo core.RepoRef, ref string) string {
	return fmt.Sprintf("tree:%s:%s", repo, ref)
}

func allKey(repo core.RepoRef, ref string) string {
	return fmt.Sprintf("all:%s:%s", repo, ref)
}

func blobKey(repo core.RepoRef, sha string) string {
	return fmt.Sprintf("blob:%s:%s", repo, sha)
}

func repoTag(repo core.RepoRef) string {
	return "repo:" + repo.String()
}
