package files

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// listing returns a recursive listing of n files under a path of pathLen
// bytes each, as GitHub sends one: a fresh string for every field.
func listing(sha string, n, pathLen int) core.Tree {
	entries := make([]core.TreeEntry, n)
	for i := range entries {
		p := fmt.Sprintf("%0*d", pathLen, i)
		entries[i] = core.TreeEntry{
			Path: p, Name: p, Type: core.EntryBlob, Mode: "100644",
			SHA: strings.Repeat("a", 40), Size: 10,
		}
	}
	return core.Tree{SHA: sha, Entries: entries}
}

func TestTreeSize(t *testing.T) {
	empty := treeSize(core.Tree{SHA: rootSHA})
	one := treeSize(listing(rootSHA, 1, 10))
	if got, want := one-empty, treeEntrySize+10+4+6+40; got != want {
		t.Errorf("an entry costs %d, want %d", got, want)
	}
	// A listing of a large repository is measured at what it takes: about
	// 18MiB for 100,000 paths of 50 bytes.
	big := treeSize(listing(rootSHA, 100_000, 50))
	if big < 15<<20 || big > 20<<20 {
		t.Errorf("100,000 entries cost %d bytes, want 15MiB to 20MiB", big)
	}
}

// TestTreeMemoryBound reads more listings than the bound holds: the least
// recently read go, from the listings by ref and by SHA alike, and what is
// kept stays within the bound.
func TestTreeMemoryBound(t *testing.T) {
	const n = 1000
	per := treeSize(listing("x", n, 20))
	bound := 3 * per
	api := &fakeAPI{t: t, treeAll: func(ref string, _ github.Conditional) (core.Tree, github.Response, error) {
		return listing("sha-"+ref, n, 20), github.Response{ETag: `"` + ref + `"`}, nil
	}}
	s := New(api, WithTreeMemory(bound))
	refs := []string{"r1", "r2", "r3", "r4", "r5"}
	for _, ref := range refs {
		if _, err := s.All(t.Context(), TreeQuery{Repo: repo, Ref: ref}); err != nil {
			t.Fatalf("All %s: %v", ref, err)
		}
	}
	for name, size := range map[string]int64{"refs": s.refs.Size(), "objects": s.objects.Size()} {
		if size > bound {
			t.Errorf("%s keep %d bytes, over the bound of %d", name, size, bound)
		}
	}
	if _, ok := s.CachedAll(TreeQuery{Repo: repo, Ref: "r1"}); ok {
		t.Error("the listing read first is still kept")
	}
	if _, ok := s.CachedAll(TreeQuery{Repo: repo, Ref: "r5"}); !ok {
		t.Error("the listing read last is gone")
	}
}

// TestTreeOverBoundKept keeps a listing larger than the bound on its own,
// so the finder of a large repository still has it.
func TestTreeOverBoundKept(t *testing.T) {
	api := &fakeAPI{t: t, treeAll: func(ref string, _ github.Conditional) (core.Tree, github.Response, error) {
		return listing("sha-"+ref, 100, 20), github.Response{}, nil
	}}
	s := New(api, WithTreeMemory(1))
	q := TreeQuery{Repo: repo, Ref: "main"}
	if _, err := s.All(t.Context(), q); err != nil {
		t.Fatalf("All: %v", err)
	}
	if _, ok := s.CachedAll(q); !ok {
		t.Error("a listing over the bound is not kept")
	}
}
