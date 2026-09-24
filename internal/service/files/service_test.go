package files

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

// fakeAPI answers with its func fields and records the calls. A nil field
// fails the test when it is called.
type fakeAPI struct {
	t *testing.T

	tree    func(tree string, cond github.Conditional) (core.Tree, github.Response, error)
	treeAll func(tree string, cond github.Conditional) (core.Tree, github.Response, error)
	blob    func(sha string, limit int64) (core.Blob, error)

	mu    sync.Mutex
	calls []string
}

func (f *fakeAPI) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeAPI) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
}

func (f *fakeAPI) GetTree(_ context.Context, repo core.RepoRef, tree string, cond github.Conditional) (core.Tree, github.Response, error) {
	f.record(fmt.Sprintf("tree %s %s %s", repo, tree, cond.ETag))
	if f.tree == nil {
		f.t.Error("unexpected GetTree")
		return core.Tree{}, github.Response{}, errors.New("unexpected call")
	}
	return f.tree(tree, cond)
}

func (f *fakeAPI) GetTreeRecursive(_ context.Context, repo core.RepoRef, tree string, cond github.Conditional) (core.Tree, github.Response, error) {
	f.record(fmt.Sprintf("all %s %s %s", repo, tree, cond.ETag))
	if f.treeAll == nil {
		f.t.Error("unexpected GetTreeRecursive")
		return core.Tree{}, github.Response{}, errors.New("unexpected call")
	}
	return f.treeAll(tree, cond)
}

func (f *fakeAPI) GetBlob(_ context.Context, repo core.RepoRef, sha string, limit int64) (core.Blob, error) {
	f.record(fmt.Sprintf("blob %s %s %d", repo, sha, limit))
	if f.blob == nil {
		f.t.Error("unexpected GetBlob")
		return core.Blob{}, errors.New("unexpected call")
	}
	return f.blob(sha, limit)
}

var repo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

const (
	rootSHA = "0f8e75abebff0877cae681a3d5ff31ac47f54220"
	subSHA  = "a6810531ea06dc8d46500967c6a4d2b13167e4ed"
)

func dir(name, sha string) core.TreeEntry {
	return core.TreeEntry{Path: name, Name: name, Type: core.EntryTree, Mode: "040000", SHA: sha}
}

func file(name string) core.TreeEntry {
	return core.TreeEntry{Path: name, Name: name, Type: core.EntryBlob, Mode: "100644", SHA: "b-" + name, Size: 10}
}

// rootTree is in the order git sorts entries, by bytes.
func rootTree() core.Tree {
	return core.Tree{SHA: rootSHA, Entries: []core.TreeEntry{
		file("Makefile"),
		file("README.md"),
		dir("cmd", subSHA),
		{Path: "docs", Name: "docs", Type: core.EntryCommit, Mode: "160000", SHA: "c-docs"},
		file("go.mod"),
		dir("Internal", "d-internal"),
	}}
}

func names(t core.Tree) []string {
	out := make([]string, 0, len(t.Entries))
	for _, e := range t.Entries {
		out = append(out, e.Name)
	}
	return out
}

func TestTreeSortsDirectoriesFirst(t *testing.T) {
	api := &fakeAPI{t: t, tree: func(string, github.Conditional) (core.Tree, github.Response, error) {
		return rootTree(), github.Response{ETag: `"e1"`}, nil
	}}
	s := New(api)

	got, err := s.Tree(t.Context(), TreeQuery{Repo: repo})
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	want := []string{"cmd", "docs", "Internal", "go.mod", "Makefile", "README.md"}
	if !slices.Equal(names(got), want) {
		t.Errorf("names = %q, want %q", names(got), want)
	}
	api.wantCalls(t, "tree eggzec/gh-tui HEAD ")
}

func TestTreeFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{t: t, tree: func(string, github.Conditional) (core.Tree, github.Response, error) {
		return rootTree(), github.Response{ETag: `"e1"`}, nil
	}}
	s := New(api)

	if _, ok := s.CachedTree(TreeQuery{Repo: repo, Ref: "main"}); ok {
		t.Error("CachedTree hit before any Tree")
	}
	for range 2 {
		if _, err := s.Tree(t.Context(), TreeQuery{Repo: repo, Ref: "main"}); err != nil {
			t.Fatalf("Tree: %v", err)
		}
	}
	api.wantCalls(t, "tree eggzec/gh-tui main ")

	if got, ok := s.CachedTree(TreeQuery{Repo: repo, Ref: "main"}); !ok || got.SHA != rootSHA {
		t.Errorf("CachedTree = %+v, %v; want the root", got, ok)
	}
	if _, ok := s.CachedTree(TreeQuery{Repo: repo, Ref: "dev"}); ok {
		t.Error("another ref hit the cache")
	}
	if _, ok := s.CachedTree(TreeQuery{Repo: core.RepoRef{Owner: "o", Name: "r"}, Ref: "main"}); ok {
		t.Error("another repository hit the cache")
	}
}

func TestTreeByRefIsCachedBySHA(t *testing.T) {
	api := &fakeAPI{t: t, tree: func(string, github.Conditional) (core.Tree, github.Response, error) {
		return rootTree(), github.Response{}, nil
	}}
	s := New(api)

	if _, err := s.Tree(t.Context(), TreeQuery{Repo: repo}); err != nil {
		t.Fatalf("Tree: %v", err)
	}
	got, ok := s.CachedTree(TreeQuery{Repo: repo, Ref: rootSHA})
	if !ok || got.SHA != rootSHA {
		t.Fatalf("CachedTree by SHA = %+v, %v; want the root", got, ok)
	}
	if _, err := s.Tree(t.Context(), TreeQuery{Repo: repo, Ref: rootSHA}); err != nil {
		t.Fatalf("Tree by SHA: %v", err)
	}
	api.wantCalls(t, "tree eggzec/gh-tui HEAD ")
}

func TestTreeRefRevalidates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t, tree: func(_ string, cond github.Conditional) (core.Tree, github.Response, error) {
			if cond.ETag != "" {
				return core.Tree{}, github.Response{NotModified: true}, nil
			}
			return rootTree(), github.Response{ETag: `"e1"`}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		q := TreeQuery{Repo: repo, Ref: "main"}

		if _, err := s.Tree(t.Context(), q); err != nil {
			t.Fatalf("Tree: %v", err)
		}
		time.Sleep(time.Minute)
		got, err := s.Tree(t.Context(), q)
		if err != nil || got.SHA != rootSHA || len(got.Entries) != 6 {
			t.Fatalf("revalidated Tree = %+v, %v; want the cached root", got, err)
		}
		// Invalidate makes the ref stale again, even while it is fresh.
		s.Invalidate(repo)
		if _, err := s.Tree(t.Context(), q); err != nil {
			t.Fatalf("Tree after Invalidate: %v", err)
		}
		api.wantCalls(t,
			"tree eggzec/gh-tui main ",
			`tree eggzec/gh-tui main "e1"`,
			`tree eggzec/gh-tui main "e1"`,
		)
	})
}

func TestTreeBySHANeverGoesStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t, tree: func(string, github.Conditional) (core.Tree, github.Response, error) {
			return core.Tree{SHA: subSHA, Entries: []core.TreeEntry{file("main.go")}}, github.Response{ETag: `"e2"`}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		q := TreeQuery{Repo: repo, Ref: subSHA}

		if _, err := s.Tree(t.Context(), q); err != nil {
			t.Fatalf("Tree: %v", err)
		}
		time.Sleep(365 * 24 * time.Hour)
		s.Invalidate(repo)
		got, err := s.Tree(t.Context(), q)
		if err != nil || got.SHA != subSHA {
			t.Fatalf("Tree = %+v, %v; want the cached directory", got, err)
		}
		api.wantCalls(t, "tree eggzec/gh-tui "+subSHA+" ")
	})
}

func TestTreeError(t *testing.T) {
	api := &fakeAPI{t: t, tree: func(string, github.Conditional) (core.Tree, github.Response, error) {
		return core.Tree{}, github.Response{}, fmt.Errorf("get tree: %w", core.ErrNotFound)
	}}
	s := New(api)
	q := TreeQuery{Repo: repo, Ref: "gone"}

	if _, err := s.Tree(t.Context(), q); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Tree error = %v, want ErrNotFound", err)
	}
	if _, ok := s.CachedTree(q); ok {
		t.Error("an error was cached")
	}
	// Errors aren't cached, so the next read tries again.
	_, _ = s.Tree(t.Context(), q)
	api.wantCalls(t, "tree eggzec/gh-tui gone ", "tree eggzec/gh-tui gone ")
}

func TestAll(t *testing.T) {
	entries := []core.TreeEntry{
		{Path: "Makefile", Name: "Makefile", Type: core.EntryBlob},
		{Path: "cmd", Name: "cmd", Type: core.EntryTree},
		{Path: "cmd/main.go", Name: "main.go", Type: core.EntryBlob},
		{Path: "internal", Name: "internal", Type: core.EntryTree},
		{Path: "internal/Core.go", Name: "Core.go", Type: core.EntryBlob},
		{Path: "internal/app.go", Name: "app.go", Type: core.EntryBlob},
	}
	api := &fakeAPI{t: t, treeAll: func(string, github.Conditional) (core.Tree, github.Response, error) {
		return core.Tree{SHA: rootSHA, Entries: slices.Clone(entries), Truncated: true}, github.Response{ETag: `"e3"`}, nil
	}}
	s := New(api)
	q := TreeQuery{Repo: repo, Ref: "main"}

	if _, ok := s.CachedAll(q); ok {
		t.Error("CachedAll hit before any All")
	}
	got, err := s.All(t.Context(), q)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	paths := make([]string, 0, len(got.Entries))
	for _, e := range got.Entries {
		paths = append(paths, e.Path)
	}
	want := []string{"cmd", "cmd/main.go", "internal", "internal/app.go", "internal/Core.go", "Makefile"}
	if !slices.Equal(paths, want) || !got.Truncated {
		t.Errorf("All = %q, truncated %v; want %q, truncated", paths, got.Truncated, want)
	}
	if _, err := s.All(t.Context(), q); err != nil {
		t.Fatalf("second All: %v", err)
	}
	if c, ok := s.CachedAll(q); !ok || len(c.Entries) != len(entries) {
		t.Errorf("CachedAll = %+v, %v; want the listing", c, ok)
	}
	// The listing and one level are separate entries.
	if _, ok := s.CachedTree(q); ok {
		t.Error("All filled the one-level entry")
	}
	api.wantCalls(t, "all eggzec/gh-tui main ")
}

func TestBlob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t, blob: func(sha string, _ int64) (core.Blob, error) {
			return core.Blob{SHA: sha, Size: 5, Content: []byte("hello")}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		q := BlobQuery{Repo: repo, SHA: "b1", Size: 5}

		if _, ok := s.CachedBlob(q); ok {
			t.Error("CachedBlob hit before any Blob")
		}
		got, err := s.Blob(t.Context(), q)
		if err != nil || string(got.Content) != "hello" {
			t.Fatalf("Blob = %+v, %v; want hello", got, err)
		}
		time.Sleep(365 * 24 * time.Hour)
		s.Invalidate(repo)
		if _, err := s.Blob(t.Context(), BlobQuery{Repo: repo, SHA: "b1"}); err != nil {
			t.Fatalf("second Blob: %v", err)
		}
		if c, ok := s.CachedBlob(BlobQuery{Repo: repo, SHA: "b1"}); !ok || string(c.Content) != "hello" {
			t.Errorf("CachedBlob = %+v, %v; want hello", c, ok)
		}
		api.wantCalls(t, fmt.Sprintf("blob eggzec/gh-tui b1 %d", DefaultMaxBlobSize))
	})
}

func TestBlobBinary(t *testing.T) {
	api := &fakeAPI{t: t, blob: func(sha string, _ int64) (core.Blob, error) {
		return core.Blob{SHA: sha, Size: 3, Content: []byte{0x89, 0, 1}, Binary: true}, nil
	}}
	s := New(api)

	got, err := s.Blob(t.Context(), BlobQuery{Repo: repo, SHA: "png"})
	if err != nil || !got.Binary {
		t.Errorf("Blob = %+v, %v; want a binary blob", got, err)
	}
}

func TestBlobTooLarge(t *testing.T) {
	t.Run("known size", func(t *testing.T) {
		api := &fakeAPI{t: t}
		s := New(api, WithMaxBlobSize(100))

		_, err := s.Blob(t.Context(), BlobQuery{Repo: repo, SHA: "big", Size: 101})
		if e, ok := errors.AsType[*core.TooLargeError](err); !ok || e.Size != 101 || e.Limit != 100 {
			t.Errorf("Blob error = %v, want a TooLargeError of 101 over 100", err)
		}
		api.wantCalls(t)
	})
	t.Run("from GitHub", func(t *testing.T) {
		api := &fakeAPI{t: t, blob: func(_ string, limit int64) (core.Blob, error) {
			return core.Blob{}, &core.TooLargeError{Limit: limit}
		}}
		s := New(api, WithMaxBlobSize(100))
		q := BlobQuery{Repo: repo, SHA: "big"}

		if _, err := s.Blob(t.Context(), q); !errors.Is(err, core.ErrTooLarge) {
			t.Errorf("Blob error = %v, want ErrTooLarge", err)
		}
		if _, ok := s.CachedBlob(q); ok {
			t.Error("a too large blob was cached")
		}
		api.wantCalls(t, "blob eggzec/gh-tui big 100")
	})
}

func TestCompareFold(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"a", "B", -1},
		{"B", "a", 1},
		// Names that differ only in case fall back to bytes.
		{"ABC", "abc", -1},
		{"abc", "ABC", 1},
		{"same", "same", 0},
		{"a", "ab", -1},
		{"Ärger", "ärger", -1},
		{"z", "é", -1},
	}
	for _, tt := range tests {
		if got := compareFold(tt.a, tt.b); got != tt.want {
			t.Errorf("compareFold(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsSHA(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{rootSHA, true},
		{"0F8E75ABEBFF0877CAE681A3D5FF31AC47F54220", true},
		{"9006fe5b82537c10729c8e8d95046abfb899d9967713b3b58e07938315a57eb8", true},
		{"HEAD", false},
		{"main", false},
		{"0f8e75a", false},
		{"0f8e75abebff0877cae681a3d5ff31ac47f5422g", false},
	}
	for _, tt := range tests {
		if got := isSHA(tt.ref); got != tt.want {
			t.Errorf("isSHA(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}
