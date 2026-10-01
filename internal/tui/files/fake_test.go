package files

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
)

// fake is a Service over fixed trees, keyed by repository and ref. Trees
// that were read once are cached, as the service caches them.
type fake struct {
	mu     sync.Mutex
	trees  map[string]core.Tree
	errs   map[string]error
	cached map[string]bool
	// reads lists the queries Tree was called with, in order; cancelled
	// counts those whose context was done by then.
	reads       []filesvc.TreeQuery
	cancelled   int
	invalidated []core.RepoRef

	// allReads lists the queries All was called with. The listing of a
	// repository at a ref is its trees walked from the tree of the ref,
	// cut short for the repositories in truncated. cachedAll holds the
	// treeKey of those whose listing is fresh, until invalidated.
	allReads  []filesvc.TreeQuery
	truncated map[string]bool
	cachedAll map[string]bool
	// offline holds the repositories whose listing comes from the disk,
	// as if GitHub couldn't be reached, and limited those whose listing
	// is kept as if GitHub rate limited the read. A kept listing stays
	// stale, so the next read asks GitHub again.
	offline, limited map[string]bool
	version          int

	// blobs and blobErrs are keyed by SHA. blobReads lists the queries
	// Blob was called with, and cachedBlobs the SHAs it returned.
	blobs       map[string]core.Blob
	blobErrs    map[string]error
	blobReads   []filesvc.BlobQuery
	cachedBlobs map[string]bool
	// onBlob, if set, runs at the start of every Blob, outside the lock,
	// so it may block.
	onBlob func(ctx context.Context, q filesvc.BlobQuery)
}

func newFake() *fake {
	return &fake{
		trees: map[string]core.Tree{}, errs: map[string]error{}, cached: map[string]bool{},
		truncated: map[string]bool{}, cachedAll: map[string]bool{}, offline: map[string]bool{}, limited: map[string]bool{},
		blobs: map[string]core.Blob{}, blobErrs: map[string]error{}, cachedBlobs: map[string]bool{},
	}
}

func treeKey(repo core.RepoRef, ref string) string {
	if ref == "" {
		ref = "HEAD"
	}
	return strings.ToLower(repo.String()) + "@" + ref
}

// addTree lists entries under ref of repo.
func (f *fake) addTree(repo core.RepoRef, ref string, entries ...core.TreeEntry) {
	f.trees[treeKey(repo, ref)] = core.Tree{SHA: ref, Entries: entries}
	// Any change gives the root, and so the listing, a new SHA.
	f.version++
}

func (f *fake) CachedTree(q filesvc.TreeQuery) (core.Tree, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := treeKey(q.Repo, q.Ref)
	if !f.cached[k] {
		return core.Tree{}, false
	}
	return f.trees[k], true
}

var errNoTree = errors.New("404 Not Found")

func (f *fake) Tree(ctx context.Context, q filesvc.TreeQuery) (core.Tree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, q)
	if err := ctx.Err(); err != nil {
		f.cancelled++
		return core.Tree{}, err
	}
	k := treeKey(q.Repo, q.Ref)
	if err := f.errs[k]; err != nil {
		return core.Tree{}, err
	}
	t, ok := f.trees[k]
	if !ok {
		return core.Tree{}, errNoTree
	}
	f.cached[k] = true
	return t, nil
}

// listing walks the trees of repo from the tree of ref. A truncated
// listing keeps the top level and drops the rest. f.mu must be held.
func (f *fake) listing(repo core.RepoRef, ref string) (core.Tree, error) {
	root := treeKey(repo, ref)
	if err := f.errs[root]; err != nil {
		return core.Tree{}, err
	}
	t, ok := f.trees[root]
	if !ok {
		return core.Tree{}, errNoTree
	}
	cut := f.truncated[strings.ToLower(repo.String())]
	name := strings.ToLower(repo.String())
	out := core.Tree{SHA: fmt.Sprintf("all-%s-%s-%d", repo, ref, f.version), Truncated: cut, Offline: f.offline[name], Limited: f.limited[name]}
	var walk func(dir string, t core.Tree)
	walk = func(dir string, t core.Tree) {
		for _, e := range t.Entries {
			e.Path = joinPath(dir, e.Name)
			out.Entries = append(out.Entries, e)
			if sub, ok := f.trees[treeKey(repo, e.SHA)]; ok && e.Dir() && !cut {
				walk(e.Path, sub)
			}
		}
	}
	walk("", t)
	return out, nil
}

func (f *fake) CachedAll(q filesvc.TreeQuery) (core.Tree, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cachedAll[treeKey(q.Repo, q.Ref)] {
		return core.Tree{}, false
	}
	t, err := f.listing(q.Repo, q.Ref)
	return t, err == nil
}

func (f *fake) All(ctx context.Context, q filesvc.TreeQuery) (core.Tree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := treeKey(q.Repo, q.Ref)
	if f.cachedAll[k] {
		// A fresh listing is served from the cache.
		return f.listing(q.Repo, q.Ref)
	}
	f.allReads = append(f.allReads, q)
	if err := ctx.Err(); err != nil {
		f.cancelled++
		return core.Tree{}, err
	}
	t, err := f.listing(q.Repo, q.Ref)
	if err != nil {
		return core.Tree{}, err
	}
	if !t.Offline && !t.Limited {
		f.cachedAll[k] = true
	}
	return t, nil
}

// allCount returns how many times All asked GitHub.
func (f *fake) allCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.allReads)
}

func (f *fake) Invalidate(repo core.RepoRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated = append(f.invalidated, repo)
	for k := range f.cachedAll {
		if strings.HasPrefix(k, strings.ToLower(repo.String())+"@") {
			delete(f.cachedAll, k)
		}
	}
}

// allRefs returns the refs All asked GitHub for, in order.
func (f *fake) allRefs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	refs := make([]string, len(f.allReads))
	for i, q := range f.allReads {
		refs[i] = q.Ref
	}
	return refs
}

// readRefs returns the refs Tree was asked for, in order.
func (f *fake) readRefs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	refs := make([]string, len(f.reads))
	for i, q := range f.reads {
		refs[i] = q.Ref
	}
	return refs
}

// blobSHAs returns the SHAs Blob was asked for, sorted, since reads ahead
// run concurrently.
func (f *fake) blobSHAs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	shas := make([]string, len(f.blobReads))
	for i, q := range f.blobReads {
		shas[i] = q.SHA
	}
	slices.Sort(shas)
	return shas
}

// addBlob stores the content of the file e.
func (f *fake) addBlob(e core.TreeEntry, content string) {
	b := []byte(content)
	f.blobs[e.SHA] = core.Blob{SHA: e.SHA, Size: int64(len(b)), Content: b, Binary: core.LooksBinary(b)}
}

func (f *fake) CachedBlob(q filesvc.BlobQuery) (core.Blob, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cachedBlobs[q.SHA] {
		return core.Blob{}, false
	}
	return f.blobs[q.SHA], true
}

func (f *fake) Blob(ctx context.Context, q filesvc.BlobQuery) (core.Blob, error) {
	if f.onBlob != nil {
		f.onBlob(ctx, q)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blobReads = append(f.blobReads, q)
	if err := ctx.Err(); err != nil {
		return core.Blob{}, err
	}
	if err := f.blobErrs[q.SHA]; err != nil {
		return core.Blob{}, fmt.Errorf("get blob %s: %w", q.SHA, err)
	}
	b, ok := f.blobs[q.SHA]
	if !ok {
		return core.Blob{}, errNoTree
	}
	f.cachedBlobs[q.SHA] = true
	return b, nil
}
