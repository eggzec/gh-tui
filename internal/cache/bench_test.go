package cache

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
)

func benchKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "repos/eggzec/gh-tui/pulls/" + strconv.Itoa(i)
	}
	return keys
}

func filled(keys []string) *Cache[int] {
	c := New[int](WithCapacity(len(keys)))
	for i, k := range keys {
		c.Set(k, Entry[int]{Value: i})
	}
	return c
}

func BenchmarkGet(b *testing.B) {
	keys := benchKeys(1024)
	c := filled(keys)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		c.Get(keys[i%len(keys)])
		i++
	}
}

func BenchmarkGetParallel(b *testing.B) {
	keys := benchKeys(1024)
	c := filled(keys)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(keys[i%len(keys)])
			i++
		}
	})
}

func BenchmarkSet(b *testing.B) {
	// Twice the capacity, so half the Sets update and half evict.
	keys := benchKeys(2048)
	c := New[int](WithCapacity(1024))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		c.Set(keys[i%len(keys)], Entry[int]{Value: i})
		i++
	}
}

func BenchmarkFetchHit(b *testing.B) {
	keys := benchKeys(1024)
	c := filled(keys)
	ctx := b.Context()
	fn := func(context.Context, Entry[int], bool) (Entry[int], error) {
		b.Fatal("fn called on a fresh hit")
		return Entry[int]{}, nil
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, err := c.Fetch(ctx, keys[i%len(keys)], fn); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

// issuePage is a page of 50 issues as a list shows them, with a few labels
// each and bodies of about a paragraph.
func issuePage() core.Page[core.Issue] {
	at := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	p := core.Page[core.Issue]{Next: "https://api.github.com/repositories/1/issues?page=2&per_page=50"}
	for i := range 50 {
		p.Items = append(p.Items, core.Issue{
			ID:        "I_kwDOAbCdEf" + strconv.Itoa(i),
			Repo:      core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"},
			Number:    1000 + i,
			Title:     "Rendering glitches when the terminal is resized quickly " + strconv.Itoa(i),
			Body:      strings.Repeat("Steps to reproduce: resize the window while a spinner runs. ", 8),
			State:     core.StateOpen,
			Author:    core.User{Login: "octocat"},
			Labels:    []core.Label{{Name: "bug", Color: "d73a4a"}, {Name: "needs-triage", Color: "ededed"}},
			Comments:  i % 7,
			CreatedAt: at.Add(-time.Duration(i) * time.Hour),
			UpdatedAt: at.Add(-time.Duration(i) * time.Minute),
			URL:       "https://github.com/charmbracelet/bubbletea/issues/" + strconv.Itoa(1000+i),
		})
	}
	return p
}

func benchShelf(b *testing.B, store Store) {
	b.Helper()
	s := NewShelf[core.Page[core.Issue]](store, "issuelist", 1)
	e := Entry[core.Page[core.Issue]]{Value: issuePage(), ETag: `W/"0123456789abcdef"`, Tags: []string{"repo:charmbracelet/bubbletea"}}
	b.Run("Save", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := s.Save("list", e); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("Load", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, ok := s.Load("list"); !ok {
				b.Fatal("miss")
			}
		}
	})
}

// BenchmarkShelfCodec measures the encoding alone.
func BenchmarkShelfCodec(b *testing.B) {
	benchShelf(b, newMemStore())
}

// BenchmarkShelfDisk measures a disk store with its default compression.
func BenchmarkShelfDisk(b *testing.B) {
	store, err := disk.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	benchShelf(b, store)
}

// BenchmarkShelfKept lists the entries of a disk store of a few thousand
// issue pages, as a revalidation pass does: first when every entry is
// read, and then when none changed since.
func BenchmarkShelfKept(b *testing.B) {
	store, err := disk.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	const n = 3000
	e := Entry[core.Page[core.Issue]]{Value: issuePage(), ETag: `W/"0123456789abcdef"`, Tags: []string{"repo:charmbracelet/bubbletea"}}
	save := NewShelf[core.Page[core.Issue]](store, "issuelist", 1)
	for i := range n {
		if err := save.Save("list:charmbracelet/bubbletea:open:30:"+strconv.Itoa(i), e); err != nil {
			b.Fatal(err)
		}
	}
	count := func(s *Shelf[core.Page[core.Issue]]) {
		got := 0
		for range s.Kept() {
			got++
		}
		if got != n {
			b.Fatalf("listed %d entries, want %d", got, n)
		}
	}
	b.Run("cold", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			count(NewShelf[core.Page[core.Issue]](store, "issuelist", 1))
		}
	})
	b.Run("warm", func(b *testing.B) {
		s := NewShelf[core.Page[core.Issue]](store, "issuelist", 1)
		count(s)
		b.ReportAllocs()
		for b.Loop() {
			count(s)
		}
	})
}
