package files

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// memStore keeps objects in a map, as the disk store keeps them in files.
type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	// fail makes every Put fail.
	fail bool
}

func newMemStore() *memStore {
	return &memStore{objects: make(map[string][]byte)}
}

func (m *memStore) Get(kind, key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[kind+"/"+key]
	return b, ok
}

func (m *memStore) Put(kind, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("disk full")
	}
	m.objects[kind+"/"+key] = slices.Clone(data)
	return nil
}

func (m *memStore) Delete(kind, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, kind+"/"+key)
}

func (m *memStore) has(kind, key string) bool {
	_, ok := m.Get(kind, key)
	return ok
}

func (m *memStore) keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Sorted(maps.Keys(m.objects))
}

// Commit SHAs: GitHub names a listing read by a ref after its commit.
const (
	commitA = "657aaae216e3f87df09b60959a9fe5de7b2ca419"
	commitB = "1111111111111111111111111111111111111111"
	// helloSHA is the blob of "hello".
	helloSHA = "b6fc4c620b67d95f953a5c1c1230aaab5db5a1b0"
)

func listingAt(commit string) core.Tree {
	return core.Tree{SHA: commit, Entries: []core.TreeEntry{
		{Path: "cmd", Name: "cmd", Type: core.EntryTree, Mode: "040000", SHA: subSHA},
		{Path: "cmd/main.go", Name: "main.go", Type: core.EntryBlob, Mode: "100644", SHA: helloSHA, Size: 5},
		{Path: "README.md", Name: "README.md", Type: core.EntryBlob, Mode: "100644", SHA: helloSHA, Size: 5},
	}}
}

// server answers the listing of HEAD like GitHub: with the listing at
// commit and its ETag, or a 304 when the request carries that ETag.
func server(commit *string) func(string, github.Conditional) (core.Tree, github.Response, error) {
	return func(_ string, cond github.Conditional) (core.Tree, github.Response, error) {
		etag := `"` + *commit + `"`
		if cond.ETag == etag {
			return core.Tree{}, github.Response{NotModified: true}, nil
		}
		return listingAt(*commit), github.Response{ETag: etag}, nil
	}
}

func hello(sha string, _ int64) (core.Blob, error) {
	return core.Blob{SHA: sha, Size: 5, Content: []byte("hello")}, nil
}

var (
	offline = &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("dial tcp: no route to host")}
	head    = TreeQuery{Repo: repo}
)

func TestStoreColdWritesThrough(t *testing.T) {
	commit := commitA
	store := newMemStore()
	api := &fakeAPI{t: t, treeAll: server(&commit), blob: hello}
	s := New(api, WithStore(store))

	got, err := s.All(t.Context(), head)
	if err != nil || got.SHA != commitA || len(got.Entries) != 3 {
		t.Fatalf("All = %+v, %v; want the listing at %s", got, err, commitA)
	}
	if _, err := s.Blob(t.Context(), BlobQuery{Repo: repo, SHA: helloSHA}); err != nil {
		t.Fatalf("Blob: %v", err)
	}
	want := []string{
		"blob/" + helloSHA,
		"listing/" + commitA,
		"ref/" + refKey(kindListing, repo, "HEAD"),
	}
	if got := store.keys(); !slices.Equal(got, want) {
		t.Errorf("store = %q, want %q", got, want)
	}
	api.wantCalls(t,
		"all eggzec/gh-tui HEAD ",
		fmt.Sprintf("blob eggzec/gh-tui %s %d", helloSHA, DefaultMaxBlobSize),
	)
}

// TestStoreWarmStart has a second session read what the first did: it asks
// GitHub only whether HEAD moved, which a 304 answers for free.
func TestStoreWarmStart(t *testing.T) {
	commit := commitA
	store := newMemStore()
	first := New(&fakeAPI{t: t, treeAll: server(&commit), tree: func(string, github.Conditional) (core.Tree, github.Response, error) {
		return core.Tree{SHA: subSHA, Entries: []core.TreeEntry{file("main.go")}}, github.Response{}, nil
	}, blob: hello}, WithStore(store))
	want, err := first.All(t.Context(), head)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if _, err := first.Tree(t.Context(), TreeQuery{Repo: repo, Ref: subSHA}); err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if _, err := first.Blob(t.Context(), BlobQuery{Repo: repo, SHA: helloSHA}); err != nil {
		t.Fatalf("Blob: %v", err)
	}

	api := &fakeAPI{t: t, treeAll: server(&commit)}
	s := New(api, WithStore(store))
	got, err := s.All(t.Context(), head)
	if err != nil || !slices.Equal(got.Entries, want.Entries) || got.SHA != want.SHA || got.Offline {
		t.Fatalf("warm All = %+v, %v; want %+v", got, err, want)
	}
	// The listing is also in memory under its commit.
	if _, ok := s.CachedAll(TreeQuery{Repo: repo, Ref: commitA}); !ok {
		t.Error("CachedAll by the commit missed")
	}
	sub, err := s.Tree(t.Context(), TreeQuery{Repo: repo, Ref: subSHA})
	if err != nil || len(sub.Entries) != 1 || sub.Entries[0].Name != "main.go" {
		t.Errorf("warm Tree = %+v, %v; want the stored directory", sub, err)
	}
	b, err := s.Blob(t.Context(), BlobQuery{Repo: repo, SHA: helloSHA})
	if err != nil || string(b.Content) != "hello" || b.Size != 5 || b.Binary {
		t.Errorf("warm Blob = %+v, %v; want hello", b, err)
	}
	// Another repository shares the content by its SHA.
	if _, err := s.Blob(t.Context(), BlobQuery{Repo: core.RepoRef{Owner: "o", Name: "fork"}, SHA: helloSHA}); err != nil {
		t.Errorf("Blob of a fork: %v", err)
	}
	api.wantCalls(t, `all eggzec/gh-tui HEAD "`+commitA+`"`)
}

func TestStoreRefMoved(t *testing.T) {
	commit := commitA
	store := newMemStore()
	if _, err := New(&fakeAPI{t: t, treeAll: server(&commit)}, WithStore(store)).All(t.Context(), head); err != nil {
		t.Fatalf("All: %v", err)
	}

	commit = commitB
	api := &fakeAPI{t: t, treeAll: server(&commit)}
	got, err := New(api, WithStore(store)).All(t.Context(), head)
	if err != nil || got.SHA != commitB {
		t.Fatalf("All = %+v, %v; want the listing at %s", got, err, commitB)
	}
	// Both listings stay, since each names its content for good.
	if !store.has(kindListing, commitA) || !store.has(kindListing, commitB) {
		t.Errorf("store = %q, want both listings", store.keys())
	}
	// A third session finds HEAD where the second left it.
	third := &fakeAPI{t: t, treeAll: server(&commit)}
	if _, err := New(third, WithStore(store)).All(t.Context(), head); err != nil {
		t.Fatalf("All: %v", err)
	}
	api.wantCalls(t, `all eggzec/gh-tui HEAD "`+commitA+`"`)
	third.wantCalls(t, `all eggzec/gh-tui HEAD "`+commitB+`"`)
}

// TestStoreRefWithoutListing makes the request unconditional when the
// listing the ref pointed at is gone, since a 304 would leave nothing.
func TestStoreRefWithoutListing(t *testing.T) {
	commit := commitA
	store := newMemStore()
	if _, err := New(&fakeAPI{t: t, treeAll: server(&commit)}, WithStore(store)).All(t.Context(), head); err != nil {
		t.Fatalf("All: %v", err)
	}
	store.Delete(kindListing, commitA)

	api := &fakeAPI{t: t, treeAll: server(&commit)}
	if got, err := New(api, WithStore(store)).All(t.Context(), head); err != nil || got.SHA != commitA {
		t.Fatalf("All = %+v, %v; want the listing", got, err)
	}
	api.wantCalls(t, "all eggzec/gh-tui HEAD ")
	if !store.has(kindListing, commitA) {
		t.Error("the listing wasn't stored again")
	}
}

func TestStoreOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		commit := commitA
		store := newMemStore()
		if _, err := New(&fakeAPI{t: t, treeAll: server(&commit)}, WithStore(store)).All(t.Context(), head); err != nil {
			t.Fatalf("All: %v", err)
		}

		reachable := false
		api := &fakeAPI{t: t, treeAll: func(ref string, cond github.Conditional) (core.Tree, github.Response, error) {
			if !reachable {
				return core.Tree{}, github.Response{}, fmt.Errorf("get tree: %w", offline)
			}
			return server(&commit)(ref, cond)
		}}
		s := New(api, WithStore(store), WithTTL(time.Hour))
		got, err := s.All(t.Context(), head)
		if err != nil || !got.Offline || got.SHA != commitA || len(got.Entries) != 3 {
			t.Fatalf("offline All = %+v, %v; want the stored listing, offline", got, err)
		}
		if c, ok := s.CachedAll(head); !ok || !c.Offline {
			t.Errorf("CachedAll = %+v, %v; want the offline listing", c, ok)
		}
		// What was served offline is stale, so the next read tries again,
		// long before the TTL.
		reachable = true
		got, err = s.All(t.Context(), head)
		if err != nil || got.Offline || got.SHA != commitA {
			t.Fatalf("All once back online = %+v, %v; want the listing, online", got, err)
		}
		if _, err := s.All(t.Context(), head); err != nil {
			t.Fatalf("All: %v", err)
		}
		etag := `"` + commitA + `"`
		api.wantCalls(t, "all eggzec/gh-tui HEAD "+etag, "all eggzec/gh-tui HEAD "+etag)
	})
}

func TestStoreNoFallback(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback bool
	}{
		{"not found", &github.Error{StatusCode: 404}, false},
		{"unauthorized", &github.Error{StatusCode: 401}, false},
		{"forbidden", &github.Error{StatusCode: 403}, false},
		{"rate limited", &github.Error{StatusCode: 429}, false},
		{"server error", &github.Error{StatusCode: 502}, true},
		{"unreachable", offline, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commit := commitA
			store := newMemStore()
			if _, err := New(&fakeAPI{t: t, treeAll: server(&commit)}, WithStore(store)).All(t.Context(), head); err != nil {
				t.Fatalf("All: %v", err)
			}
			api := &fakeAPI{t: t, treeAll: func(string, github.Conditional) (core.Tree, github.Response, error) {
				return core.Tree{}, github.Response{}, fmt.Errorf("get tree: %w", tt.err)
			}}
			got, err := New(api, WithStore(store)).All(t.Context(), head)
			if tt.fallback {
				if err != nil || !got.Offline || len(got.Entries) == 0 {
					t.Errorf("All = %+v, %v; want the stored listing", got, err)
				}
				return
			}
			if err == nil || len(got.Entries) != 0 {
				t.Errorf("All = %+v, %v; want the error and nothing stored", got, err)
			}
		})
	}
}

func TestOfflineWithoutStore(t *testing.T) {
	api := &fakeAPI{t: t, treeAll: func(string, github.Conditional) (core.Tree, github.Response, error) {
		return core.Tree{}, github.Response{}, offline
	}}
	if _, err := New(api).All(t.Context(), head); !errors.Is(err, offline) {
		t.Errorf("All error = %v, want the network error", err)
	}
}

func TestStoreOneLevelByRef(t *testing.T) {
	store := newMemStore()
	root := func(_ string, cond github.Conditional) (core.Tree, github.Response, error) {
		if cond.ETag == `"t1"` {
			return core.Tree{}, github.Response{NotModified: true}, nil
		}
		return core.Tree{SHA: commitA, Entries: []core.TreeEntry{file("go.mod"), dir("cmd", subSHA)}}, github.Response{ETag: `"t1"`}, nil
	}
	if _, err := New(&fakeAPI{t: t, tree: root}, WithStore(store)).Tree(t.Context(), head); err != nil {
		t.Fatalf("Tree: %v", err)
	}
	api := &fakeAPI{t: t, tree: root}
	got, err := New(api, WithStore(store)).Tree(t.Context(), head)
	if err != nil || !slices.Equal(names(got), []string{"cmd", "go.mod"}) {
		t.Fatalf("warm Tree = %q, %v; want the sorted root", names(got), err)
	}
	api.wantCalls(t, `tree eggzec/gh-tui HEAD "t1"`)
	if !store.has(kindTree, commitA) || store.has(kindListing, commitA) {
		t.Errorf("store = %q, want the root as a tree only", store.keys())
	}
}

func TestStoreCorruptTree(t *testing.T) {
	commit := commitA
	store := newMemStore()
	if _, err := New(&fakeAPI{t: t, treeAll: server(&commit)}, WithStore(store)).All(t.Context(), TreeQuery{Repo: repo, Ref: commitA}); err != nil {
		t.Fatalf("All: %v", err)
	}
	data, _ := store.Get(kindListing, commitA)
	if err := store.Put(kindListing, commitA, data[:len(data)-3]); err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{t: t, treeAll: server(&commit)}
	if got, err := New(api, WithStore(store)).All(t.Context(), TreeQuery{Repo: repo, Ref: commitA}); err != nil || len(got.Entries) != 3 {
		t.Fatalf("All = %+v, %v; want the listing from GitHub", got, err)
	}
	api.wantCalls(t, "all eggzec/gh-tui "+commitA+" ")
	if data, _ := store.Get(kindListing, commitA); len(data) == 0 {
		t.Error("the listing wasn't stored again")
	}
}

func TestStoreCorruptBlob(t *testing.T) {
	store := newMemStore()
	if err := store.Put(kindBlob, helloSHA, []byte("jello")); err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{t: t, blob: hello}
	b, err := New(api, WithStore(store)).Blob(t.Context(), BlobQuery{Repo: repo, SHA: helloSHA})
	if err != nil || string(b.Content) != "hello" {
		t.Fatalf("Blob = %+v, %v; want hello from GitHub", b, err)
	}
	if got, _ := store.Get(kindBlob, helloSHA); string(got) != "hello" {
		t.Errorf("stored blob = %q, want hello", got)
	}
	api.wantCalls(t, fmt.Sprintf("blob eggzec/gh-tui %s %d", helloSHA, DefaultMaxBlobSize))
}

func TestStoreBlobNotNamedByContent(t *testing.T) {
	store := newMemStore()
	// GitHub would never answer so, but the store mustn't keep content
	// under a name that isn't its hash.
	api := &fakeAPI{t: t, blob: func(sha string, _ int64) (core.Blob, error) {
		return core.Blob{SHA: sha, Content: []byte("not hello")}, nil
	}}
	if _, err := New(api, WithStore(store)).Blob(t.Context(), BlobQuery{Repo: repo, SHA: helloSHA}); err != nil {
		t.Fatalf("Blob: %v", err)
	}
	if keys := store.keys(); len(keys) != 0 {
		t.Errorf("store = %q, want nothing", keys)
	}
}

func TestStoreBlobOverLimit(t *testing.T) {
	store := newMemStore()
	if err := store.Put(kindBlob, helloSHA, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	// A session with a lower limit doesn't show what a session with a
	// higher one kept.
	api := &fakeAPI{t: t, blob: func(_ string, limit int64) (core.Blob, error) {
		return core.Blob{}, &core.TooLargeError{Limit: limit}
	}}
	if _, err := New(api, WithStore(store), WithMaxBlobSize(4)).Blob(t.Context(), BlobQuery{Repo: repo, SHA: helloSHA}); !errors.Is(err, core.ErrTooLarge) {
		t.Errorf("Blob error = %v, want ErrTooLarge", err)
	}
}

func TestStorePutFails(t *testing.T) {
	commit := commitA
	store := newMemStore()
	store.fail = true
	api := &fakeAPI{t: t, treeAll: server(&commit), blob: hello}
	s := New(api, WithStore(store))
	if _, err := s.All(t.Context(), head); err != nil {
		t.Errorf("All: %v", err)
	}
	if _, err := s.Blob(t.Context(), BlobQuery{Repo: repo, SHA: helloSHA}); err != nil {
		t.Errorf("Blob: %v", err)
	}
}

func TestBlobMemory(t *testing.T) {
	api := &fakeAPI{t: t, blob: func(sha string, _ int64) (core.Blob, error) {
		return core.Blob{SHA: sha, Content: make([]byte, 100)}, nil
	}}
	s := New(api, WithBlobMemory(2*(100+64)))
	for _, sha := range []string{"b1", "b2", "b3"} {
		if _, err := s.Blob(t.Context(), BlobQuery{Repo: repo, SHA: sha}); err != nil {
			t.Fatalf("Blob: %v", err)
		}
	}
	for sha, want := range map[string]bool{"b1": false, "b2": true, "b3": true} {
		if _, ok := s.CachedBlob(BlobQuery{Repo: repo, SHA: sha}); ok != want {
			t.Errorf("CachedBlob(%s) = %v, want %v", sha, ok, want)
		}
	}
}

func TestCodecTree(t *testing.T) {
	for _, tree := range []core.Tree{
		listingAt(commitA),
		{SHA: commitB, Truncated: true},
		{SHA: subSHA, Entries: []core.TreeEntry{file("main.go"), {Path: "docs", Name: "docs", Type: core.EntryCommit, Mode: "160000", SHA: "c-docs"}}},
	} {
		data := encodeTree(tree)
		got, err := decodeTree(data)
		if err != nil {
			t.Fatalf("decodeTree: %v", err)
		}
		if got.SHA != tree.SHA || got.Truncated != tree.Truncated || !slices.Equal(got.Entries, tree.Entries) {
			t.Errorf("decodeTree(encodeTree(%+v)) = %+v", tree, got)
		}
		// Every shorter object, or one with more after it, is damaged.
		for n := range data {
			if _, err := decodeTree(data[:n]); err == nil {
				t.Errorf("decodeTree of %d of %d bytes succeeded", n, len(data))
			}
		}
		if _, err := decodeTree(append(data, 0)); err == nil {
			t.Error("decodeTree with a byte too many succeeded")
		}
	}
	if _, err := decodeTree([]byte{codecVersion + 1, 0, 0, 0}); err == nil {
		t.Error("decodeTree of another version succeeded")
	}
}

func TestCodecRef(t *testing.T) {
	r := refRecord{SHA: commitA, ETag: `W/"abc"`, LastModified: "Thu, 17 Sep 2026 08:08:29 GMT"}
	data := encodeRef(r)
	if got, err := decodeRef(data); err != nil || got != r {
		t.Errorf("decodeRef = %+v, %v; want %+v", got, err, r)
	}
	for n := range data {
		if _, err := decodeRef(data[:n]); err == nil {
			t.Errorf("decodeRef of %d of %d bytes succeeded", n, len(data))
		}
	}
	if _, err := decodeRef(encodeRef(refRecord{SHA: "main"})); err == nil {
		t.Error("decodeRef of a ref that isn't a SHA succeeded")
	}
}

func TestRefKey(t *testing.T) {
	k := refKey(kindListing, repo, "main")
	if !isSHA(k) || len(k) != 64 {
		t.Errorf("refKey = %q, want 64 hex digits", k)
	}
	if refKey(kindListing, core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}, "main") != k {
		t.Error("refKey depends on the case of the repository")
	}
	for _, other := range []string{
		refKey(kindListing, repo, "Main"),
		refKey(kindTree, repo, "main"),
		refKey(kindListing, core.RepoRef{Owner: "eggzec", Name: "other"}, "main"),
	} {
		if other == k {
			t.Error("refKey is the same for another ref, kind or repository")
		}
	}
}

func TestIsBlobID(t *testing.T) {
	tests := []struct {
		sha     string
		content string
		want    bool
	}{
		{helloSHA, "hello", true},
		{"4b5fa63702dd96796042e92787f464e28f09f17d", "hello, world\n", true},
		{"8aec4e4876f854f688d0ebfc8f37598f38e5fd6903cccc850ca36591175aeb60", "hello", true},
		{helloSHA, "hello\n", false},
		{"8aec4e4876f854f688d0ebfc8f37598f38e5fd6903cccc850ca36591175aeb61", "hello", false},
		{"b6fc4c62", "hello", false},
		{"B6FC4C620B67D95F953A5C1C1230AAAB5DB5A1B0", "hello", true},
	}
	for _, tt := range tests {
		if got := isBlobID(tt.sha, []byte(tt.content)); got != tt.want {
			t.Errorf("isBlobID(%s, %q) = %v, want %v", tt.sha, tt.content, got, tt.want)
		}
	}
}
