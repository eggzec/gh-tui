package files

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
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

// prefetching reads ahead as the defaults do, with the file under the
// cursor read up to 1MiB, after edit changes the settings, if set.
func prefetching(edit func(p *config.PrefetchLayers)) Option {
	p := config.Default().Prefetch
	if edit != nil {
		edit(&p)
	}
	return WithPrefetch(p, config.MiB)
}

// window reads the files after files in a window of after rows below the
// cursor, the row under it, of at most maxSize bytes, once the cursor
// rests for rest.
func window(after int, maxSize config.Size, rest time.Duration) Option {
	return prefetching(func(p *config.PrefetchLayers) {
		p.Files.Preview.Window = config.Span{Before: new(0), After: new(after)}
		p.Files.Preview.MaxSize = maxSize
		p.Files.Rest = new(rest)
	})
}

func TestPrefetchWindow(t *testing.T) {
	f := prefetchFake()
	s := newSection(t, f, 40, 12, WithRepo(ghTUI), window(32, 1000, 0))
	// Only the files of at most 1000 bytes that are likely text, as the
	// tree shows them: not AGENTS.md or the README, which are larger, not
	// the link, the submodule or the image, and not cmd/gh-tui/main.go,
	// whose folder is closed.
	want := []string{"b-.gitignore", "b-empty.txt", "b-go.mod"}
	if got := f.blobSHAs(); !slices.Equal(got, want) {
		t.Errorf("read ahead %q, want %q", got, want)
	}
	if _, ok := f.CachedBlob(filesvc.BlobQuery{Repo: ghTUI, SHA: "b-go.mod"}); !ok {
		t.Error("go.mod isn't cached after reading ahead")
	}

	// A refresh that finds the same listing reads again only .gitignore,
	// which failed: a refresh tries again what failed.
	keys(s, "r")
	want = []string{"b-.gitignore", "b-.gitignore", "b-empty.txt", "b-go.mod"}
	if got := f.blobSHAs(); !slices.Equal(got, want) {
		t.Errorf("read ahead %q after a refresh, want %q", got, want)
	}
	// A new listing reads only what isn't cached: the new file, and
	// .gitignore again.
	root := f.trees[treeKey(ghTUI, "")]
	added := file("new.md", 10)
	f.addBlob(added, "new")
	f.addTree(ghTUI, "", append(root.Entries, added)...)
	keys(s, "r")
	want = []string{"b-.gitignore", "b-.gitignore", "b-.gitignore", "b-empty.txt", "b-go.mod", "b-new.md"}
	if got := f.blobSHAs(); !slices.Equal(got, want) {
		t.Errorf("read ahead %q after a change, want %q", got, want)
	}
}

// filesFake lists n small files at the top level of gh-tui.
func filesFake(n int) *fake {
	f := newFake()
	entries := make([]core.TreeEntry, n)
	for i := range entries {
		entries[i] = file(fmt.Sprintf("f%02d.go", i), 10)
		f.addBlob(entries[i], "package f")
	}
	f.addTree(ghTUI, "", entries...)
	return f
}

func TestPrefetchWindowIsCapped(t *testing.T) {
	f := filesFake(60)
	// The defaults read the file under the cursor and the 32 below it.
	newSection(t, f, 40, 12, WithRepo(ghTUI), prefetching(func(p *config.PrefetchLayers) { p.Files.Rest = new(time.Duration(0)) }))
	got := f.blobSHAs()
	if len(got) != 33 || got[0] != "b-f00.go" || got[len(got)-1] != "b-f32.go" {
		t.Errorf("read ahead %q, want the first 33 files", got)
	}
}

func TestPrefetchOff(t *testing.T) {
	for name, opts := range map[string][]Option{
		"without WithPrefetch": nil,
		"disabled":             {prefetching(func(p *config.PrefetchLayers) { p.Files.Preview.Enabled = new(false) })},
		"all disabled":         {prefetching(func(p *config.PrefetchLayers) { p.Enabled = false })},
	} {
		t.Run(name, func(t *testing.T) {
			f := prefetchFake()
			s := loaded(t, f, 40, 12, opts...)
			keys(s, slices.Repeat([]string{"down"}, rowAgents)...)
			if got := f.blobSHAs(); len(got) != 0 {
				t.Errorf("read ahead %q", got)
			}
		})
	}
}

func TestPrefetchWaitsForTheCursorToRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const rest = 150 * time.Millisecond
		f := filesFake(4)
		// The list loading counts as a rest.
		s := newSection(t, f, 40, 12, WithRepo(ghTUI), window(0, 1000, rest))
		if got := f.blobSHAs(); !slices.Equal(got, []string{"b-f00.go"}) {
			t.Fatalf("read %q at the start, want the file under the cursor", got)
		}
		wait := s.Update(press("down"))
		if got := f.blobSHAs(); len(got) != 1 {
			t.Fatalf("read %q before the rest", got)
		}
		start := time.Now()
		run(s, wait)
		if d := time.Since(start); d != rest {
			t.Errorf("read after %v, want %v", d, rest)
		}
		if got := f.blobSHAs(); !slices.Equal(got, []string{"b-f00.go", "b-f01.go"}) {
			t.Errorf("read %q, want f01.go next", got)
		}

		// Passing a file and resting on the next reads only that one.
		passed := s.Update(press("down"))
		rested := s.Update(press("down"))
		run(s, passed)
		run(s, rested)
		if got := f.blobSHAs(); !slices.Equal(got, []string{"b-f00.go", "b-f01.go", "b-f03.go"}) {
			t.Errorf("read %q, want f03.go alone after f01.go", got)
		}
		// Coming back to a cached file reads nothing.
		keys(s, "up", "up")
		if got := f.blobSHAs(); !slices.Equal(got, []string{"b-f00.go", "b-f01.go", "b-f02.go", "b-f03.go"}) {
			t.Errorf("read %q, want only f02.go, which was passed", got)
		}
	})
}

// A rate limit stops the reads ahead until GitHub answers again.
func TestPrefetchResumesWhenTheRateLimitLifts(t *testing.T) {
	f := filesFake(6)
	f.blobErrs["b-f00.go"] = core.ErrRateLimited
	s := newSection(t, f, 40, 12, WithRepo(ghTUI), window(0, 1000, 0))
	keys(s, "down")
	if got := f.blobSHAs(); slices.Contains(got, "b-f01.go") {
		t.Fatalf("read %q under the rate limit", got)
	}
	delete(f.blobErrs, "b-f00.go")
	run(s, s.Update(ui.OnlineMsg{}))
	keys(s, "down")
	if got := f.blobSHAs(); !slices.Contains(got, "b-f02.go") {
		t.Errorf("read %q once the rate limit lifted, want f02.go", got)
	}
}

func TestPrefetchCancelsTheLastRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := filesFake(3)
		s := newSection(t, f, 40, 12, WithRepo(ghTUI), window(0, 1000, time.Millisecond))
		started := make(chan context.Context, 1)
		var lastErr error
		f.onBlob = func(ctx context.Context, q filesvc.BlobQuery) {
			if q.SHA == "b-f01.go" {
				// Hang until cancelled.
				started <- ctx
				<-ctx.Done()
				return
			}
			lastErr = ctx.Err()
		}
		wait := s.Update(press("down"))
		read := s.Update(wait())
		if read == nil {
			t.Fatal("resting on f01.go read nothing")
		}
		done := make(chan struct{})
		go func() {
			read()
			close(done)
		}()
		first := <-started

		// Resting on f02.go reads it and cancels the read of f01.go.
		keys(s, "down")
		<-done
		if first.Err() == nil {
			t.Error("the read of f01.go wasn't cancelled")
		}
		if lastErr != nil {
			t.Errorf("the read of f02.go ran with %v", lastErr)
		}
	})
}

func TestPrefetchCursorReadsUpToThePreview(t *testing.T) {
	f := sampleFake()
	// AGENTS.md is larger than the window reads, but the preview reads
	// it, so it is read once the cursor rests on it.
	s := newSection(t, f, 40, 12, WithRepo(ghTUI), window(0, 1000, 0))
	keys(s, slices.Repeat([]string{"down"}, rowAgents)...)
	if got := f.blobSHAs(); !slices.Contains(got, "b-AGENTS.md") {
		t.Errorf("read %q, want AGENTS.md under the cursor", got)
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
	s := newSection(t, f, 40, 12, WithRepo(ghTUI), window(32, 1000, 0))
	run(s, s.Update(ui.RepoMsg{Repo: other}))
	mu.Lock()
	defer mu.Unlock()
	if len(ctxs) < 3 {
		t.Fatalf("%d reads ahead, want 3 of gh-tui", len(ctxs))
	}
	for i, ctx := range ctxs[:3] {
		if ctx.Err() == nil {
			t.Errorf("read ahead %d still runs after another repository was selected", i)
		}
	}
}

func TestPrefetchFolders(t *testing.T) {
	f := sampleFake()
	f.truncated["eggzec/gh-tui"] = true
	tree := prefetching(func(p *config.PrefetchLayers) {
		p.Files.Tree.Enabled = new(true)
		p.Files.Tree.Window = config.Span{Before: new(0), After: new(1)}
		p.Files.Preview.Enabled = new(false)
	})
	newSection(t, f, 40, 12, WithRepo(ghTUI), tree)
	// The listing was too large to read at once, so the folders in the
	// window are listed before they are opened.
	refs := make([]string, 0, len(f.reads))
	for _, q := range f.reads {
		refs = append(refs, q.Ref)
	}
	for _, want := range []string{cmdSHA, internalSHA} {
		if !slices.Contains(refs, want) {
			t.Errorf("listed %q, want %s listed ahead", refs, want)
		}
	}
}

func TestPrefetchFoldersOfAWholeListing(t *testing.T) {
	f := sampleFake()
	tree := prefetching(func(p *config.PrefetchLayers) { p.Files.Tree.Enabled = new(true) })
	newSection(t, f, 40, 12, WithRepo(ghTUI), tree)
	// A whole listing opens its folders without requests.
	if len(f.reads) != 0 {
		t.Errorf("listed %v, want nothing", f.reads)
	}
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
	// A limit of 0 reads nothing, not even an empty file.
	if worthReading(file("Makefile", 0), 0) {
		t.Error("worthReading(an empty file, 0) = true, want false")
	}
}

func TestPreviewOfPrefetchedFile(t *testing.T) {
	f := sampleFake()
	h := newHost(newSection(t, f, 40, 12, WithRepo(ghTUI), window(32, 20_000, 0)))
	h.keys(slices.Repeat([]string{"down"}, rowAgents)...)
	before := len(f.blobSHAs())
	// Fail any read, so that only the cache can show the file.
	f.onBlob = func(context.Context, filesvc.BlobQuery) { t.Error("the preview read a prefetched file again") }
	h.keys("enter")
	if got := h.modal(); !strings.Contains(got, "Guidance for anyone.") {
		t.Errorf("preview = %q, want AGENTS.md", got)
	}
	if n := len(f.blobSHAs()); n != before {
		t.Errorf("%d reads after the preview, want %d", n, before)
	}
}
