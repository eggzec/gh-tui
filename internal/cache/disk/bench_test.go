package disk

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// source returns n bytes of the Go source of this module, which compresses
// like most files in a repository.
func source(b *testing.B, n int) []byte {
	b.Helper()
	names, err := filepath.Glob("../../*/*.go")
	if err != nil {
		b.Fatal(err)
	}
	var src []byte
	for _, name := range names {
		f, err := os.ReadFile(name)
		if err != nil {
			b.Fatal(err)
		}
		if src = append(src, f...); len(src) >= n {
			return src[:n]
		}
	}
	b.Fatalf("only %d bytes of source", len(src))
	return nil
}

// listing returns n lines shaped like the entries of a recursive tree
// listing: a path, a mode, a SHA and a size.
func listing(n int) []byte {
	var sb strings.Builder
	for i := range n {
		fmt.Fprintf(&sb, "src/pkg%d/sub%d/file_%d.go\x00100644\x00%040x\x00%d\n", i/500, i/50, i, i*7919, i%9000)
	}
	return []byte(sb.String())
}

var levels = []struct {
	name  string
	level int
}{
	{"none", gzip.NoCompression},
	{"fastest", gzip.BestSpeed},
	{"default", gzip.DefaultCompression},
	{"best", gzip.BestCompression},
}

func benchPut(b *testing.B, data []byte) {
	b.Helper()
	for _, l := range levels {
		b.Run(l.name, func(b *testing.B) {
			s, err := Open(b.TempDir(), WithCompression(l.level))
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if err := s.Put("blob", key, data); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(diskSize(b, s)), "disk-B")
		})
	}
}

func benchGet(b *testing.B, data []byte) {
	b.Helper()
	for _, l := range levels {
		b.Run(l.name, func(b *testing.B) {
			s, err := Open(b.TempDir(), WithCompression(l.level))
			if err != nil {
				b.Fatal(err)
			}
			if err := s.Put("blob", key, data); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if _, ok := s.Get("blob", key); !ok {
					b.Fatal("miss")
				}
			}
			b.ReportMetric(float64(diskSize(b, s)), "disk-B")
		})
	}
}

func diskSize(b *testing.B, s *Store) int64 {
	b.Helper()
	u, err := s.Collect(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	return u.Size
}

func BenchmarkPutBlob64KiB(b *testing.B)         { benchPut(b, source(b, 64<<10)) }
func BenchmarkGetBlob64KiB(b *testing.B)         { benchGet(b, source(b, 64<<10)) }
func BenchmarkPutListing50kEntries(b *testing.B) { benchPut(b, listing(50_000)) }
func BenchmarkGetListing50kEntries(b *testing.B) { benchGet(b, listing(50_000)) }
