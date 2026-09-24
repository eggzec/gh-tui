// Package files browses the files of GitHub repositories: one directory of a
// tree at a time, every path of a tree at once for search, and the content of
// a file. Git objects never change once they exist, so whatever is read by
// SHA stays cached for good; only what a ref such as a branch points at is
// revalidated.
//
// Reads go to memory first, then to the Store, if there is one, and then to
// GitHub, and what GitHub returns is kept in both. The store also keeps where
// each ref pointed and its validators, so a new session asks GitHub whether a
// ref moved with a conditional request, which costs no rate limit when it
// didn't, and reads the rest from the store.
package files

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
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
	store   Store
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
	o := options{
		blobCapacity: DefaultBlobCapacity,
		blobMemory:   DefaultBlobMemory,
		maxBlob:      DefaultMaxBlobSize,
		store:        noStore{},
	}
	for _, opt := range opts {
		opt(&o)
	}
	immutable := slices.Concat(o.cache, []cache.Option{cache.WithTTL(forever)})
	return &Service{
		api:     api,
		store:   o.store,
		maxBlob: o.maxBlob,
		refs:    cache.New[core.Tree](o.cache...),
		objects: cache.New[core.Tree](immutable...),
		blobs: cache.New[core.Blob](
			cache.WithCapacity(o.blobCapacity),
			cache.WithMaxSize(o.blobMemory, blobSize),
			cache.WithTTL(forever),
		),
	}
}

// blobSize is what a cached blob costs in memory, roughly.
func blobSize(b core.Blob) int64 {
	return int64(len(b.Content)) + 64
}

// Invalidate marks the trees that refs of repo point at stale, so the next
// read asks GitHub whether the refs moved. It costs no rate limit if they
// didn't. Trees and blobs read by SHA can't change and are kept.
func (s *Service) Invalidate(repo core.RepoRef) {
	s.refs.InvalidateTag(repoTag(repo))
}

// unreachable reports whether err means that GitHub couldn't be reached or
// failed, rather than refused: only then may a ref fall back to where it
// last pointed. An account that lost access to a repository gets a 401, 403
// or 404 and never sees what an earlier session cached.
func unreachable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if e, ok := errors.AsType[*github.Error](err); ok {
		return e.StatusCode >= 500
	}
	_, ok := errors.AsType[*url.Error](err)
	return ok
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
