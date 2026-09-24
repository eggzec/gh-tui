package files

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// children lists the directories of repo for the tree. Every node carries
// its core.TreeEntry, with Path relative to the root of the repository.
func children(svc Service, repo core.RepoRef) tree.Children {
	return func(ctx context.Context, parent tree.Node) ([]tree.Node, error) {
		dir, _ := parent.Value.(core.TreeEntry)
		t, err := readTree(ctx, svc, filesvc.TreeQuery{Repo: repo, Ref: dir.SHA})
		if err != nil {
			return nil, err
		}
		nodes := make([]tree.Node, len(t.Entries))
		for i, e := range t.Entries {
			// A listing of one level holds names only.
			e.Path = path.Join(dir.Path, e.Name)
			nodes[i] = tree.Node{ID: e.Path, Name: e.Name, Branch: e.Dir(), Value: e}
		}
		return nodes, nil
	}
}

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

// webURL returns the page of e on GitHub, or of the repository when e is
// the zero entry. Files are blobs; directories and submodules are trees.
func webURL(repo core.RepoRef, e core.TreeEntry) string {
	u := "https://github.com/" + escapePath(repo.String())
	if e.Path == "" {
		return u
	}
	kind := "/blob/HEAD/"
	if e.Dir() || e.Submodule() {
		kind = "/tree/HEAD/"
	}
	return u + kind + escapePath(e.Path)
}

// escapePath escapes each segment of p, keeping the slashes between them.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
