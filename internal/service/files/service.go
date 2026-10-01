// Package files browses the files of GitHub repositories: one directory of a
// tree at a time, every path of a tree at once for search, and the content of
// a file. Git objects never change once they exist, so whatever is read by
// SHA stays cached for good; only what a ref such as a branch points at is
// revalidated.
//
// Reads go to memory first, then to the Store, if there is one, and then to
// GitHub, and what GitHub returns is kept in both. The store also keeps where
// each ref pointed, its validators and when GitHub last confirmed it, so a
// new session asks GitHub whether a ref moved with a conditional request,
// which costs no rate limit when it didn't, unless GitHub confirmed it within
// the TTL, and reads the rest from the store.
package files

import (
	"cmp"
	"context"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// API is the part of the GitHub client that the service uses.
type API interface {
	GetTree(ctx context.Context, repo core.RepoRef, tree string, cond github.Conditional) (core.Tree, github.Response, error)
	GetTreeRecursive(ctx context.Context, repo core.RepoRef, tree string, cond github.Conditional) (core.Tree, github.Response, error)
	GetBlob(ctx context.Context, repo core.RepoRef, sha string, limit int64) (core.Blob, error)
}

// Service reads trees and blobs. It is safe for concurrent use.
type Service struct {
	api     API
	store   Store
	maxBlob int64
	// ttl is how long a tree read by a ref stays fresh.
	ttl time.Duration
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
	o := options{store: noStore{}}
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	ttl := cmp.Or(o.ttl, d.Cache.TTL.Files)
	capacity := cache.WithCapacity(cmp.Or(o.capacity, d.Cache.Memory.Entries))
	// What a SHA names never changes, so it never goes stale.
	return &Service{
		api:     api,
		store:   o.store,
		maxBlob: cmp.Or(o.maxBlob, int64(d.Files.Preview.MaxSize)),
		ttl:     ttl,
		refs:    cache.New[core.Tree](cache.WithTTL(ttl), capacity),
		objects: cache.New[core.Tree](capacity),
		blobs: cache.New[core.Blob](
			cache.WithCapacity(cmp.Or(o.blobCapacity, d.Cache.Memory.Entries)),
			cache.WithMaxSize(cmp.Or(o.blobMemory, int64(d.Cache.Memory.Files)), blobSize),
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

// Cache keys and tags.

func treeKey(repo core.RepoRef, ref string) string {
	return "tree:" + repoID(repo) + ":" + ref
}

func allKey(repo core.RepoRef, ref string) string {
	return "all:" + repoID(repo) + ":" + ref
}

func blobKey(repo core.RepoRef, sha string) string {
	return "blob:" + repoID(repo) + ":" + sha
}

func repoTag(repo core.RepoRef) string {
	return "repo:" + repoID(repo)
}

// repoID names a repository in keys and tags. GitHub ignores case in owner
// and repository names, so keys do too.
func repoID(r core.RepoRef) string {
	return strings.ToLower(r.String())
}
