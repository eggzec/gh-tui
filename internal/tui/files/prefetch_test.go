package files

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// prefetchFake is sampleFake with a small image and an empty file at the
// top level, and their contents.
func prefetchFake() *fake {
	f := sampleFake()
	root := f.trees[treeKey(ghTUI, "")]
	logo, empty := file("logo.PNG", 100), file("empty.txt", 0)
	root.Entries = append(root.Entries, logo, empty)
	f.addTree(ghTUI, "", root.Entries...)
	f.addBlob(logo, "\x89PNG")
	f.addBlob(empty, "")
	return f
}

func TestPrefetchTopLevel(t *testing.T) {
	f := prefetchFake()
	s := newSection(t, f, 40, 12, WithRepo(ghTUI), WithPrefetch(1000))
	// Only the top-level files of at most 1000 bytes that are likely text:
	// not AGENTS.md or the README, which are larger, not the link, the
	// submodule or the image, and not cmd/gh-tui/main.go, which is nested.
	want := []string{"b-.gitignore", "b-empty.txt", "b-go.mod"}
	if got := f.blobSHAs(); !slices.Equal(got, want) {
		t.Errorf("read ahead %q, want %q", got, want)
	}
	if _, ok := f.CachedBlob(filesvc.BlobQuery{Repo: ghTUI, SHA: "b-go.mod"}); !ok {
		t.Error("go.mod isn't cached after reading ahead")
	}

	// A refresh that finds the same listing reads nothing again.
	keys(s, "r")
	if got := f.blobSHAs(); !slices.Equal(got, want) {
		t.Errorf("read ahead %q after a refresh, want nothing new", got)
	}
	// A new listing reads only what isn't cached: the new file, and
	// .gitignore, which failed.
	root := f.trees[treeKey(ghTUI, "")]
	added := file("new.md", 10)
	f.addBlob(added, "new")
	f.addTree(ghTUI, "", append(root.Entries, added)...)
	keys(s, "r")
	want = []string{"b-.gitignore", "b-.gitignore", "b-empty.txt", "b-go.mod", "b-new.md"}
	if got := f.blobSHAs(); !slices.Equal(got, want) {
		t.Errorf("read ahead %q after a change, want %q", got, want)
	}
}

func TestPrefetchIsCapped(t *testing.T) {
	f := newFake()
	entries := make([]core.TreeEntry, 60)
	for i := range entries {
		entries[i] = file(fmt.Sprintf("f%02d.go", i), 10)
		f.addBlob(entries[i], "package f")
	}
	f.addTree(ghTUI, "", entries...)
	newSection(t, f, 40, 12, WithRepo(ghTUI), WithPrefetch(1000))
	got := f.blobSHAs()
	if len(got) != prefetchFiles || got[0] != "b-f00.go" || got[len(got)-1] != fmt.Sprintf("b-f%02d.go", prefetchFiles-1) {
		t.Errorf("read ahead %q, want the first %d files", got, prefetchFiles)
	}
}

func TestPrefetchOff(t *testing.T) {
	f := prefetchFake()
	loaded(t, f, 40, 12)
	if got := f.blobSHAs(); len(got) != 0 {
		t.Errorf("read ahead %q without WithPrefetch", got)
	}
}

func TestPrefetchCancelledOnRepoChange(t *testing.T) {
	f := prefetchFake()
	var mu sync.Mutex
	var ctxs []context.Context
	f.onBlob = func(ctx context.Context, _ filesvc.BlobQuery) {
		mu.Lock()
		defer mu.Unlock()
		ctxs = append(ctxs, ctx)
	}
	s := newSection(t, f, 40, 12, WithRepo(ghTUI), WithPrefetch(1000))
	run(s, s.Update(ui.RepoMsg{Repo: other}))
	mu.Lock()
	defer mu.Unlock()
	if len(ctxs) != 3 {
		t.Fatalf("%d reads ahead, want 3", len(ctxs))
	}
	for i, ctx := range ctxs[:3] {
		if ctx.Err() == nil {
			t.Errorf("read ahead %d still runs after another repository was selected", i)
		}
	}
}

func TestReadAllIsBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		var mu sync.Mutex
		inFlight, most := 0, 0
		f.onBlob = func(context.Context, filesvc.BlobQuery) {
			mu.Lock()
			inFlight++
			most = max(most, inFlight)
			mu.Unlock()
			time.Sleep(time.Second)
			mu.Lock()
			inFlight--
			mu.Unlock()
		}
		qs := make([]filesvc.BlobQuery, 12)
		for i := range qs {
			qs[i] = filesvc.BlobQuery{Repo: ghTUI, SHA: strconv.Itoa(i)}
		}
		start := time.Now()
		readAll(t.Context(), f, nil, qs, 4)
		if most != 4 || len(f.blobReads) != 12 {
			t.Errorf("%d reads, at most %d at once; want 12, at most 4", len(f.blobReads), most)
		}
		if d := time.Since(start); d != 3*time.Second {
			t.Errorf("took %v, want 3 rounds of 4", d)
		}
	})
}

func TestReadAllStopsWhenCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		ctx, cancel := context.WithCancel(t.Context())
		f.onBlob = func(ctx context.Context, _ filesvc.BlobQuery) {
			cancel()
			<-ctx.Done()
		}
		qs := make([]filesvc.BlobQuery, 12)
		for i := range qs {
			qs[i] = filesvc.BlobQuery{Repo: ghTUI, SHA: strconv.Itoa(i)}
		}
		readAll(ctx, f, nil, qs, 4)
		// The reads already started finish; no new one starts.
		if n := len(f.blobReads); n > 4 {
			t.Errorf("%d reads after the first was cancelled, want at most 4", n)
		}
	})
}

func TestWorthReading(t *testing.T) {
	link := file("link", 5)
	link.Mode = core.ModeSymlink
	tests := []struct {
		e    core.TreeEntry
		want bool
	}{
		{file("main.go", 64), true},
		{file("Makefile", 0), true},
		{file("big.go", 65), false},
		{file("font.woff2", 10), false},
		{file("Photo.JPG", 10), false},
		{link, false},
		{dir("cmd", cmdSHA), false},
		{core.TreeEntry{Name: "sub", Type: core.EntryCommit, Mode: "160000"}, false},
	}
	for _, tt := range tests {
		if got := worthReading(tt.e, 64); got != tt.want {
			t.Errorf("worthReading(%s) = %v, want %v", tt.e.Name, got, tt.want)
		}
	}
}

func TestPreviewOfPrefetchedFile(t *testing.T) {
	f := sampleFake()
	h := newHost(newSection(t, f, 40, 12, WithRepo(ghTUI), WithPrefetch(20_000)))
	before := len(f.blobSHAs())
	// Fail any read, so that only the cache can show the file.
	f.onBlob = func(context.Context, filesvc.BlobQuery) { t.Error("the preview read a prefetched file again") }
	h.keys(slices.Repeat([]string{"down"}, rowAgents)...)
	h.keys("enter")
	if got := h.modal(); !strings.Contains(got, "Guidance for anyone.") {
		t.Errorf("preview = %q, want AGENTS.md", got)
	}
	if n := len(f.blobSHAs()); n != before {
		t.Errorf("%d reads after the preview, want %d", n, before)
	}
}
