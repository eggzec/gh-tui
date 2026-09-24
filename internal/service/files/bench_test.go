package files

import (
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
)

// bigListing is shaped like the listing of a large repository.
func bigListing(n int) core.Tree {
	t := core.Tree{SHA: commitA, Entries: make([]core.TreeEntry, n)}
	for i := range t.Entries {
		p := fmt.Sprintf("src/pkg%d/sub%d/file_%d.go", i/500, i/50, i)
		h := sha1.Sum([]byte(p))
		t.Entries[i] = core.TreeEntry{
			Path: p, Name: fmt.Sprintf("file_%d.go", i), Type: core.EntryBlob, Mode: "100644",
			SHA: hex.EncodeToString(h[:]), Size: int64(i % 9000),
		}
	}
	return t
}

func BenchmarkEncodeListing50k(b *testing.B) {
	t := bigListing(50_000)
	b.ReportAllocs()
	var n int
	for b.Loop() {
		n = len(encodeTree(t))
	}
	b.ReportMetric(float64(n), "raw-B")
}

func BenchmarkDecodeListing50k(b *testing.B) {
	data := encodeTree(bigListing(50_000))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := decodeTree(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStoredListing50k reads a listing as a new session does: from the
// disk store, decompressed and decoded.
func BenchmarkStoredListing50k(b *testing.B) {
	for _, l := range []struct {
		name  string
		level int
	}{{"none", gzip.NoCompression}, {"gzip", gzip.DefaultCompression}} {
		b.Run(l.name, func(b *testing.B) {
			store, err := disk.Open(b.TempDir(), disk.WithCompression(l.level))
			if err != nil {
				b.Fatal(err)
			}
			s := New(nil, WithStore(store))
			if !s.keep(kindListing, commitA, bigListing(50_000)) {
				b.Fatal("keep failed")
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, ok := s.stored(kindListing, commitA); !ok {
					b.Fatal("miss")
				}
			}
			u, err := store.Collect(b.Context())
			if err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(u.Size), "disk-B")
		})
	}
}
