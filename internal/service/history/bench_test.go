package history

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// bigDetail is a commit that changed 300 files, a page's worth, each with a
// patch of about a kilobyte.
func bigDetail() core.CommitDetail {
	patch := "@@ -1,20 +1,20 @@\n" + strings.Repeat(" context line of a file\n-removed line\n+added line\n", 20)
	d := core.CommitDetail{
		SHA: sha(0), Subject: "all: gofmt", Message: "all: gofmt\n\nSigned-off-by: A <a@example.com>",
		Stats: core.CommitStats{Additions: 6000, Deletions: 6000, Total: 12000},
	}
	for i := range 300 {
		d.Files = append(d.Files, core.CommitFile{
			Path:      fmt.Sprintf("src/pkg%d/file%d.go", i/10, i),
			Status:    core.FileModified,
			SHA:       sha(i),
			Additions: 20,
			Deletions: 20,
			Patch:     patch,
		})
	}
	return d
}

// BenchmarkCommitFromStore measures reading a 300-file commit that an
// earlier session kept, as a cold start does.
func BenchmarkCommitFromStore(b *testing.B) {
	f := newFake(b, 1)
	_, objects := stores(b)
	s := New(f, WithObjects(objects))
	key := detailKey(repo, sha(0))
	if err := s.keptDetails.Save(key, cacheEntry(bigDetail())); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		d, ok := s.keptDetails.Load(key)
		if !ok || len(d.Value.Files) != 300 {
			b.Fatal("missed the kept commit")
		}
	}
}

// BenchmarkCachedCommit measures reading a 300-file commit from memory, as
// every render of the History modal may.
func BenchmarkCachedCommit(b *testing.B) {
	s := New(newFake(b, 1))
	s.details.Set(detailKey(repo, sha(0)), cacheEntry(bigDetail()))
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := s.CachedCommit(repo, sha(0)); !ok {
			b.Fatal("missed the cached commit")
		}
	}
}

func cacheEntry[V any](v V) cache.Entry[V] {
	return cache.Entry[V]{Value: v}
}
