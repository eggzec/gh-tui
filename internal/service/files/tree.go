package files

import (
	"cmp"
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// TreeQuery selects a git tree of a repository.
type TreeQuery struct {
	Repo core.RepoRef
	// Ref is a tree or commit SHA, or a branch, a tag or HEAD, which name
	// the root of the commit they point at. Empty means HEAD, the default
	// branch. Trees read by SHA are cached for good; the rest are
	// revalidated after the TTL.
	Ref string
}

func (q TreeQuery) normalize() TreeQuery {
	q.Ref = cmp.Or(q.Ref, "HEAD")
	return q
}

// CachedTree returns the cached directory for q, fresh or stale, without a
// request. It reports false if the directory isn't in memory; it doesn't
// read the store.
func (s *Service) CachedTree(q TreeQuery) (core.Tree, bool) {
	return s.cached(q, level)
}

// Tree returns one level of a tree: the entries of a directory, with
// directories and submodules first, then files, each by name ignoring case.
// A tree read by a ref is also cached under its SHA, so reading it again by
// Tree.SHA makes no request.
func (s *Service) Tree(ctx context.Context, q TreeQuery) (core.Tree, error) {
	q = q.normalize()
	t, err := s.read(ctx, q, level)
	if err != nil {
		return core.Tree{}, fmt.Errorf("list tree %s of %s: %w", q.Ref, q.Repo, err)
	}
	return t, nil
}

// CachedAll returns the cached recursive listing for q, fresh or stale,
// without a request. It reports false if the listing isn't in memory; it
// doesn't read the store.
func (s *Service) CachedAll(q TreeQuery) (core.Tree, bool) {
	return s.cached(q, recursive)
}

// All returns every entry below a tree in one request, for searching by
// path, sorted by path ignoring case. Paths are relative to the tree. A very
// large tree comes back Truncated, missing entries; Tree still lists every
// directory.
func (s *Service) All(ctx context.Context, q TreeQuery) (core.Tree, error) {
	q = q.normalize()
	t, err := s.read(ctx, q, recursive)
	if err != nil {
		return core.Tree{}, fmt.Errorf("list all files of %s at %s: %w", q.Repo, q.Ref, err)
	}
	return t, nil
}

// lister is one way of listing a tree: one level of it, or all of it.
type lister struct {
	// kind is what the store keeps its listings as.
	kind string
	key  func(repo core.RepoRef, ref string) string
	get  func(api API, ctx context.Context, repo core.RepoRef, tree string, cond github.Conditional) (core.Tree, github.Response, error)
	sort func(core.TreeEntry, core.TreeEntry) int
}

var (
	level = lister{
		kind: kindTree,
		key:  treeKey,
		get:  API.GetTree,
		sort: CompareEntries,
	}
	recursive = lister{
		kind: kindListing,
		key:  allKey,
		get:  API.GetTreeRecursive,
		sort: func(a, b core.TreeEntry) int { return compareFold(a.Path, b.Path) },
	}
)

func (s *Service) cached(q TreeQuery, l lister) (core.Tree, bool) {
	q = q.normalize()
	c := s.refs
	if isSHA(q.Ref) {
		c, q.Ref = s.objects, strings.ToLower(q.Ref)
	}
	e, st := c.Get(l.key(q.Repo, q.Ref))
	return e.Value, st != cache.Miss
}

func (s *Service) read(ctx context.Context, q TreeQuery, l lister) (core.Tree, error) {
	if isSHA(q.Ref) {
		return s.object(ctx, q.Repo, strings.ToLower(q.Ref), l)
	}
	return s.byRef(ctx, q, l)
}

// object reads the tree named sha from memory, the store or GitHub. It
// never changes, so it is never revalidated.
func (s *Service) object(ctx context.Context, repo core.RepoRef, sha string, l lister) (core.Tree, error) {
	e, err := s.objects.Fetch(ctx, l.key(repo, sha), func(ctx context.Context, _ cache.Entry[core.Tree], _ bool) (cache.Entry[core.Tree], error) {
		if t, ok := s.stored(l.kind, sha); ok {
			return cache.Entry[core.Tree]{Value: t}, nil
		}
		t, _, err := l.get(s.api, ctx, repo, sha, github.Conditional{})
		if err != nil {
			return cache.Entry[core.Tree]{}, err
		}
		slices.SortFunc(t.Entries, l.sort)
		s.keep(l.kind, sha, t)
		return cache.Entry[core.Tree]{Value: t}, nil
	})
	return e.Value, err
}

// offlineAt is when a tree served offline was fetched, as far as the cache
// can tell: long ago, so it is stale at once and the next read tries GitHub
// again.
var offlineAt = time.Unix(1, 0)

// byRef reads the tree that q.Ref points at, and revalidates it once it is
// stale. On a miss in memory, the ref's last response in the store makes the
// request conditional, so a new session pays nothing for a ref that didn't
// move. If GitHub can't be reached, the tree the ref last pointed at is
// served with Offline set.
func (s *Service) byRef(ctx context.Context, q TreeQuery, l lister) (core.Tree, error) {
	tags := []string{repoTag(q.Repo)}
	rkey := refKey(l.kind, q.Repo, q.Ref)
	e, err := s.refs.Fetch(ctx, l.key(q.Repo, q.Ref), func(ctx context.Context, prev cache.Entry[core.Tree], ok bool) (cache.Entry[core.Tree], error) {
		if !ok {
			prev, ok = s.storedRef(q.Repo, rkey, l)
		}
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		t, res, err := l.get(s.api, ctx, q.Repo, q.Ref, cond)
		switch {
		case err != nil && ok && github.Unreachable(ctx, err):
			prev.Value.Offline = true
			prev.FetchedAt, prev.Tags = offlineAt, tags
			return prev, nil
		case err != nil:
			return cache.Entry[core.Tree]{}, err
		case res.NotModified && ok:
			prev.Value.Offline = false
			prev.FetchedAt, prev.Tags = time.Time{}, tags
			return prev, nil
		case res.NotModified:
			return cache.Entry[core.Tree]{}, cache.ErrNotModified
		}
		slices.SortFunc(t.Entries, l.sort)
		if isSHA(t.SHA) {
			sha := strings.ToLower(t.SHA)
			s.objects.Set(l.key(q.Repo, sha), cache.Entry[core.Tree]{Value: t})
			if s.keep(l.kind, sha, t) {
				_ = s.store.Put(kindRef, rkey, encodeRef(refRecord{SHA: sha, ETag: res.ETag, LastModified: res.LastModified}))
			}
		}
		return cache.Entry[core.Tree]{Value: t, ETag: res.ETag, LastModified: res.LastModified, Tags: tags}, nil
	})
	return e.Value, err
}

// stored returns the tree of kind named sha from the store. One that can't
// be decoded is removed, so that it is read from GitHub and stored again.
func (s *Service) stored(kind, sha string) (core.Tree, bool) {
	data, ok := s.store.Get(kind, sha)
	if !ok {
		return core.Tree{}, false
	}
	t, err := decodeTree(data)
	if err != nil {
		s.store.Delete(kind, sha)
		return core.Tree{}, false
	}
	return t, true
}

// storedRef returns the tree that the ref of rkey pointed at when it was last
// read, with that response's validators, from the store. It also puts the
// tree in memory under its SHA.
func (s *Service) storedRef(repo core.RepoRef, rkey string, l lister) (cache.Entry[core.Tree], bool) {
	data, ok := s.store.Get(kindRef, rkey)
	if !ok {
		return cache.Entry[core.Tree]{}, false
	}
	r, err := decodeRef(data)
	if err != nil {
		s.store.Delete(kindRef, rkey)
		return cache.Entry[core.Tree]{}, false
	}
	// Without the tree, a 304 would leave nothing to show, so the request
	// mustn't be conditional.
	t, ok := s.stored(l.kind, r.SHA)
	if !ok {
		return cache.Entry[core.Tree]{}, false
	}
	s.objects.Set(l.key(repo, r.SHA), cache.Entry[core.Tree]{Value: t})
	return cache.Entry[core.Tree]{Value: t, ETag: r.ETag, LastModified: r.LastModified}, true
}

// keep puts t in the store as the tree of kind named sha, and reports
// whether it did. The store is only a shortcut, so a failure is ignored.
func (s *Service) keep(kind, sha string, t core.Tree) bool {
	return s.store.Put(kind, sha, encodeTree(t)) == nil
}

// CompareEntries orders directories and submodules before files, then by
// name ignoring case, the order in which Tree lists a directory.
func CompareEntries(a, b core.TreeEntry) int {
	return cmp.Or(
		cmp.Compare(fileRank(a), fileRank(b)),
		compareFold(a.Name, b.Name),
	)
}

func fileRank(e core.TreeEntry) int {
	if e.Dir() || e.Submodule() {
		return 0
	}
	return 1
}

// compareFold compares a and b ignoring case, and falls back to their bytes
// so names that differ only in case keep a stable order. Unlike comparing
// strings.ToLower of both, it doesn't allocate, which matters when sorting
// every path of a large repository.
func compareFold(a, b string) int {
	x, y := a, b
	for x != "" && y != "" {
		ra, na := utf8.DecodeRuneInString(x)
		rb, nb := utf8.DecodeRuneInString(y)
		if c := cmp.Compare(unicode.ToLower(ra), unicode.ToLower(rb)); c != 0 {
			return c
		}
		x, y = x[na:], y[nb:]
	}
	return cmp.Or(cmp.Compare(len(x), len(y)), strings.Compare(a, b))
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
