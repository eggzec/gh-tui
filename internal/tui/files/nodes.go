package files

import (
	"context"
	"net/url"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// readTree reads a directory. Directories below the root are read by SHA,
// which names content that never changes, so a cached one is good at any
// age. The root is read by HEAD, which moves, so it goes through Tree to be
// revalidated once stale.
func readTree(ctx context.Context, svc Service, q filesvc.TreeQuery) (core.Tree, error) {
	if q.Ref != "" {
		if t, ok := svc.CachedTree(q); ok {
			return t, nil
		}
	}
	return svc.Tree(ctx, q)
}

// entryOf returns the entry a node of the tree stands for.
func entryOf(n tree.Node) (core.TreeEntry, bool) {
	e, ok := n.Value.(core.TreeEntry)
	return e, ok
}

// webURL returns the page of e at ref on GitHub, or of the repository at
// ref when e is the zero entry. Files are blobs; directories and submodules
// are trees. An empty ref is the head of the default branch.
func webURL(repo core.RepoRef, ref string, e core.TreeEntry) string {
	u := "https://github.com/" + escapePath(repo.String())
	if ref == "" {
		ref = "HEAD"
	}
	if e.Path == "" {
		if ref == "HEAD" {
			return u
		}
		return u + "/tree/" + escapePath(ref)
	}
	kind := "/blob/"
	if e.Dir() || e.Submodule() {
		kind = "/tree/"
	}
	return u + kind + escapePath(ref) + "/" + escapePath(e.Path)
}

// escapePath escapes each segment of p, keeping the slashes between them.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
