package files

import (
	"context"
	"errors"
	"fmt"
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

	// blobs and blobErrs are keyed by SHA. blobReads lists the queries
	// Blob was called with, and cachedBlobs the SHAs it returned.
	blobs       map[string]core.Blob
	blobErrs    map[string]error
	blobReads   []filesvc.BlobQuery
	cachedBlobs map[string]bool
}

func newFake() *fake {
	return &fake{
		trees: map[string]core.Tree{}, errs: map[string]error{}, cached: map[string]bool{},
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

func (f *fake) Invalidate(repo core.RepoRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated = append(f.invalidated, repo)
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
