package files

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// index holds a recursive listing of a repository by directory, so the tree
// expands without requests.
type index struct {
	sha string
	// truncated reports that GitHub cut the listing short, so directories
	// may be missing entries and are read one by one instead.
	truncated bool
	// offline reports that GitHub couldn't be reached, so the listing is
	// the one kept on disk, and limited that GitHub rate limited the read.
	offline, limited bool
	// dirs holds the entries of each directory by its path, "" for the
	// root, in the order the tree shows them.
	dirs map[string][]core.TreeEntry
}

// newIndex groups the entries of a listing by directory into one backing
// array, then sorts each directory as Tree orders one.
func newIndex(t core.Tree) *index {
	counts := make(map[string]int)
	for _, e := range t.Entries {
		counts[parentOf(e.Path)]++
	}
	// next is where the next entry of each directory goes.
	next := make(map[string]int, len(counts))
	off := 0
	for dir, n := range counts {
		next[dir] = off
		off += n
	}
	all := make([]core.TreeEntry, len(t.Entries))
	for _, e := range t.Entries {
		dir := parentOf(e.Path)
		all[next[dir]] = e
		next[dir]++
	}
	dirs := make(map[string][]core.TreeEntry, len(counts))
	for dir, end := range next {
		d := all[end-counts[dir] : end : end]
		slices.SortFunc(d, filesvc.CompareEntries)
		dirs[dir] = d
	}
	return &index{sha: t.SHA, truncated: t.Truncated, offline: t.Offline, limited: t.Limited, dirs: dirs}
}

// parentOf returns the directory of a path, "" for a top-level entry.
func parentOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

// source lists the files of one repository for the tree: from the
// recursive listing, which one request reads, or one directory at a time
// when the listing is truncated. It is safe for concurrent use, as the tree
// loads in commands.
type source struct {
	svc Service
	// host is the web host of the user's GitHub, which the nodes link to.
	host string
	repo core.RepoRef
	// ref is the base to list, or empty for the head of the default
	// branch.
	ref string

	mu sync.Mutex
	// idx is the index of the listing read last.
	idx *index
}

func newSource(svc Service, host string, repo core.RepoRef, ref string) *source {
	return &source{svc: svc, host: host, repo: repo, ref: ref}
}

// current returns the index of the listing read last, or nil.
func (s *source) current() *index {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx
}

// load reads the listing, asking GitHub whether it changed once it is
// stale, and returns its index.
func (s *source) load(ctx context.Context) (*index, error) {
	t, err := s.svc.All(ctx, filesvc.TreeQuery{Repo: s.repo, Ref: s.ref})
	if err != nil {
		return nil, err
	}
	return s.indexOf(t), nil
}

// cached returns the index of the cached listing, fresh or stale, and
// reads it only when it isn't cached.
func (s *source) cached(ctx context.Context) (*index, error) {
	if t, ok := s.svc.CachedAll(filesvc.TreeQuery{Repo: s.repo, Ref: s.ref}); ok {
		return s.indexOf(t), nil
	}
	return s.load(ctx)
}

// indexOf returns the index of t, built once per listing. The SHA of the
// root names its whole content, so an index with the same SHA is the same.
func (s *source) indexOf(t core.Tree) *index {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x := s.idx; x != nil && t.SHA != "" && x.sha == t.SHA && x.truncated == t.Truncated && x.offline == t.Offline && x.limited == t.Limited {
		return x
	}
	s.idx = newIndex(t)
	return s.idx
}

// children lists a directory for the tree. The root revalidates the
// listing, so a refresh sees new commits; directories below take the
// cached listing, so expanding never waits on GitHub.
func (s *source) children(ctx context.Context, parent tree.Node) (_ []tree.Node, err error) {
	ctx, end := obs.Begin(ctx, "files.dir")
	defer func() { end(err, "span", "tui", "repo", s.repo.String(), "ref", s.ref, "root", parent.ID == "") }()
	dir, _ := entryOf(parent)
	read := s.cached
	if parent.ID == "" {
		read = s.load
	}
	x, err := read(ctx)
	if err != nil {
		return nil, err
	}
	if x.truncated {
		return s.lazy(ctx, dir)
	}
	return s.nodes(x.dirs[dir.Path]), nil
}

// lazy reads one directory, for listings that are truncated.
func (s *source) lazy(ctx context.Context, dir core.TreeEntry) ([]tree.Node, error) {
	t, err := readTree(ctx, s.svc, filesvc.TreeQuery{Repo: s.repo, Ref: dir.SHA})
	if err != nil {
		return nil, err
	}
	entries := make([]core.TreeEntry, len(t.Entries))
	for i, e := range t.Entries {
		// A listing of one level holds names only.
		e.Path = joinPath(dir.Path, e.Name)
		entries[i] = e
	}
	return s.nodes(entries), nil
}

func joinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// nodes returns the nodes of entries. Every node carries its
// core.TreeEntry, with Path relative to the root of the repository, and a
// file its size as the detail. Each links to its page at the ref listed.
func (s *source) nodes(entries []core.TreeEntry) []tree.Node {
	out := make([]tree.Node, len(entries))
	for i, e := range entries {
		n := tree.Node{ID: e.Path, Name: e.Name, Branch: e.Dir(), Value: e, Link: webURL(s.host, s.repo, s.ref, e)}
		if e.Type == core.EntryBlob && !e.Symlink() {
			n.Detail = ui.Size(e.Size)
		}
		out[i] = n
	}
	return out
}
