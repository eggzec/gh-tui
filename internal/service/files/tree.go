package files

import (
	"cmp"
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
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

// cacheFor returns the cache that holds the trees of ref.
func (s *Service) cacheFor(ref string) *cache.Cache[core.Tree] {
	if isSHA(ref) {
		return s.objects
	}
	return s.refs
}

// CachedTree returns the cached directory for q, fresh or stale, without a
// request. It reports false if the directory isn't cached.
func (s *Service) CachedTree(q TreeQuery) (core.Tree, bool) {
	q = q.normalize()
	e, st := s.cacheFor(q.Ref).Get(treeKey(q.Repo, q.Ref))
	return e.Value, st != cache.Miss
}

// Tree returns one level of a tree: the entries of a directory, with
// directories and submodules first, then files, each by name ignoring case.
// A tree read by a ref is also cached under its SHA, so reading it again by
// Tree.SHA makes no request.
func (s *Service) Tree(ctx context.Context, q TreeQuery) (core.Tree, error) {
	q = q.normalize()
	byRef := !isSHA(q.Ref)
	t, err := fetch(ctx, s.cacheFor(q.Ref), treeKey(q.Repo, q.Ref), []string{repoTag(q.Repo)},
		func(ctx context.Context, cond github.Conditional) (core.Tree, github.Response, error) {
			t, res, err := s.api.GetTree(ctx, q.Repo, q.Ref, cond)
			if err != nil || res.NotModified {
				return t, res, err
			}
			slices.SortFunc(t.Entries, CompareEntries)
			if byRef && t.SHA != "" {
				s.objects.Set(treeKey(q.Repo, t.SHA), cache.Entry[core.Tree]{Value: t})
			}
			return t, res, nil
		})
	if err != nil {
		return core.Tree{}, fmt.Errorf("list tree %s of %s: %w", q.Ref, q.Repo, err)
	}
	return t, nil
}

// CachedAll returns the cached recursive listing for q, fresh or stale,
// without a request. It reports false if the listing isn't cached.
func (s *Service) CachedAll(q TreeQuery) (core.Tree, bool) {
	q = q.normalize()
	e, st := s.cacheFor(q.Ref).Get(allKey(q.Repo, q.Ref))
	return e.Value, st != cache.Miss
}

// All returns every entry below a tree in one request, for searching by
// path, sorted by path ignoring case. Paths are relative to the tree. A very
// large tree comes back Truncated, missing entries; Tree still lists every
// directory.
func (s *Service) All(ctx context.Context, q TreeQuery) (core.Tree, error) {
	q = q.normalize()
	t, err := fetch(ctx, s.cacheFor(q.Ref), allKey(q.Repo, q.Ref), []string{repoTag(q.Repo)},
		func(ctx context.Context, cond github.Conditional) (core.Tree, github.Response, error) {
			t, res, err := s.api.GetTreeRecursive(ctx, q.Repo, q.Ref, cond)
			if err != nil || res.NotModified {
				return t, res, err
			}
			slices.SortFunc(t.Entries, func(a, b core.TreeEntry) int {
				return compareFold(a.Path, b.Path)
			})
			return t, res, nil
		})
	if err != nil {
		return core.Tree{}, fmt.Errorf("list all files of %s at %s: %w", q.Repo, q.Ref, err)
	}
	return t, nil
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
