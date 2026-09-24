package disk

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

const key = "0f8e75abebff0877cae681a3d5ff31ac47f54220"

// text is data that compresses well, like source code.
var text = []byte(strings.Repeat("func main() { fmt.Println(\"hello, world\") }\n", 100))

func open(t *testing.T, opts ...Option) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cache"), opts...)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func objectPath(s *Store, kind, key string) string {
	return filepath.Join(s.Dir(), kind, key[:2], key)
}

func exists(t *testing.T, name string) bool {
	t.Helper()
	_, err := os.Stat(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat: %v", err)
	}
	return err == nil
}

func TestRoundTrip(t *testing.T) {
	random := make([]byte, 4096)
	_, _ = rand.Read(random)
	tests := []struct {
		name  string
		opts  []Option
		data  []byte
		gzip  bool
		empty bool
	}{
		{name: "compressed", data: text, gzip: true},
		{name: "fastest", opts: []Option{WithCompression(gzip.BestSpeed)}, data: text, gzip: true},
		{name: "uncompressed", opts: []Option{WithCompression(gzip.NoCompression)}, data: text},
		// Data that doesn't get smaller is stored as is.
		{name: "incompressible", data: random},
		{name: "empty", data: []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := open(t, tt.opts...)
			if _, ok := s.Get("blob", key); ok {
				t.Error("Get hit before Put")
			}
			if err := s.Put("blob", key, tt.data); err != nil {
				t.Fatalf("Put: %v", err)
			}
			got, ok := s.Get("blob", key)
			if !ok || !bytes.Equal(got, tt.data) {
				t.Errorf("Get = %d bytes, %v; want the %d bytes put", len(got), ok, len(tt.data))
			}
			p := objectPath(s, "blob", key)
			if exists(t, p+gzExt) != tt.gzip || exists(t, p) == tt.gzip {
				t.Errorf("gzip file exists = %v, want %v", exists(t, p+gzExt), tt.gzip)
			}
			if _, ok := s.Get("tree", key); ok {
				t.Error("another kind hit")
			}
		})
	}
}

func TestCompressedIsGzip(t *testing.T) {
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatalf("Put: %v", err)
	}
	f, err := os.Open(objectPath(s, "blob", key) + gzExt)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil || !bytes.Equal(got, text) {
		t.Errorf("gunzip = %d bytes, %v; want the text", len(got), err)
	}
}

func TestChangeCompression(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	gz, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := gz.Put("ref", key, text); err != nil {
		t.Fatalf("Put: %v", err)
	}
	plain, err := Open(dir, WithCompression(gzip.NoCompression))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// What was written compressed stays readable.
	if got, ok := plain.Get("ref", key); !ok || !bytes.Equal(got, text) {
		t.Fatalf("Get = %d bytes, %v; want the text", len(got), ok)
	}
	// Writing again replaces the object in the other format too, so the
	// store never holds two versions of one object.
	if err := plain.Put("ref", key, []byte("new")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	for _, s := range []*Store{gz, plain} {
		if got, ok := s.Get("ref", key); !ok || string(got) != "new" {
			t.Errorf("Get = %q, %v; want new", got, ok)
		}
	}
	if exists(t, objectPath(plain, "ref", key)+gzExt) {
		t.Error("the old compressed object is still there")
	}
}

func TestInvalidNames(t *testing.T) {
	s := open(t)
	for _, n := range []struct{ kind, key string }{
		{"blob", "../../etc"},
		{"blob", "abc"},
		{"blob", "0F8E"},
		{"blob", strings.Repeat("a", 129)},
		{"../x", key},
		{"Blob", key},
		{"", key},
	} {
		if err := s.Put(n.kind, n.key, text); err == nil {
			t.Errorf("Put(%q, %q) succeeded, want an error", n.kind, n.key)
		}
		if _, ok := s.Get(n.kind, n.key); ok {
			t.Errorf("Get(%q, %q) hit", n.kind, n.key)
		}
		s.Delete(n.kind, n.key)
	}
}

func TestCorruptIsMiss(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func(z []byte) []byte
	}{
		{"garbage", func([]byte) []byte { return []byte("not gzip at all") }},
		{"truncated", func(z []byte) []byte { return z[:len(z)/2] }},
		{"flipped", func(z []byte) []byte {
			z = bytes.Clone(z)
			// Past the header, so the checksum catches it.
			z[len(z)-12] ^= 0xff
			return z
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := open(t)
			if err := s.Put("blob", key, text); err != nil {
				t.Fatalf("Put: %v", err)
			}
			p := objectPath(s, "blob", key) + gzExt
			z, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, tt.corrupt(z), filePerm); err != nil {
				t.Fatal(err)
			}
			if got, ok := s.Get("blob", key); ok {
				t.Errorf("Get = %d bytes, want a miss", len(got))
			}
			if exists(t, p) {
				t.Error("the corrupt object was kept")
			}
		})
	}
}

// TestCorruptSizeHint has a damaged object claim a size of 4 GiB in its
// trailer, which must not be allocated.
func TestCorruptSizeHint(t *testing.T) {
	s := open(t)
	z, err := s.gzip(text)
	if err != nil {
		t.Fatal(err)
	}
	bad := slices.Concat(z[:len(z)/2], []byte{0xff, 0xff, 0xff, 0xff})
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if _, err := gunzip(bad); err == nil {
		t.Error("gunzip of a damaged object succeeded")
	}
	runtime.ReadMemStats(&after)
	if n := after.TotalAlloc - before.TotalAlloc; n > 16<<20 {
		t.Errorf("gunzip allocated %d bytes for a %d byte object", n, len(bad))
	}
}

func TestDelete(t *testing.T) {
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatalf("Put: %v", err)
	}
	s.Delete("blob", key)
	if _, ok := s.Get("blob", key); ok {
		t.Error("Get hit after Delete")
	}
	s.Delete("blob", key)
}

func TestPermissions(t *testing.T) {
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatalf("Put: %v", err)
	}
	for _, p := range []string{s.Dir(), filepath.Join(s.Dir(), "blob"), filepath.Dir(objectPath(s, "blob", key))} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s has mode %v, want no access for others", p, perm)
		}
	}
	fi, err := os.Stat(objectPath(s, "blob", key) + gzExt)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("object has mode %v, want no access for others", perm)
	}
}

func TestOpenFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, filePerm); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(file, "cache")); err == nil {
		t.Error("Open below a file succeeded, want an error")
	}
}

// TestConcurrentWriters has writers, in this process or others, replace one
// object while readers read it: every read sees a whole version.
func TestConcurrentWriters(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	versions := make([][]byte, 8)
	for i := range versions {
		versions[i] = bytes.Repeat([]byte{'a' + byte(i)}, 1000*(i+1))
	}
	var wg sync.WaitGroup
	for i := range versions {
		// A store per writer, as separate processes would have.
		s, err := Open(dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		wg.Go(func() {
			for range 20 {
				if err := s.Put("ref", key, versions[i]); err != nil {
					t.Errorf("Put: %v", err)
				}
			}
		})
		wg.Go(func() {
			for range 50 {
				got, ok := s.Get("ref", key)
				if ok && !isVersion(got, versions) {
					t.Errorf("Get = %d bytes of %q, want a whole version", len(got), got[:1])
				}
			}
		})
	}
	wg.Wait()
	entries, err := os.ReadDir(filepath.Join(dir, "ref", key[:2]))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Errorf("temporary file %s left behind", e.Name())
		}
	}
}

func isVersion(b []byte, versions [][]byte) bool {
	for _, v := range versions {
		if bytes.Equal(b, v) {
			return true
		}
	}
	return false
}

func TestReadTouchesOldObjects(t *testing.T) {
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatalf("Put: %v", err)
	}
	p := objectPath(s, "blob", key) + gzExt
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("blob", key); !ok {
		t.Fatal("Get missed")
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(fi.ModTime()) > time.Minute {
		t.Errorf("modification time %v wasn't updated by the read", fi.ModTime())
	}
}

func TestCollect(t *testing.T) {
	s := open(t, WithMaxSize(10_000), WithCompression(gzip.NoCompression))
	base := time.Now().Add(-10 * 24 * time.Hour)
	keys := make([]string, 12)
	for i := range keys {
		keys[i] = strings.Repeat(string("0123456789ab"[i]), 40)
		if err := s.Put("blob", keys[i], bytes.Repeat([]byte{'x'}, 1000)); err != nil {
			t.Fatalf("Put: %v", err)
		}
		// Older keys were used longer ago.
		at := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(objectPath(s, "blob", keys[i]), at, at); err != nil {
			t.Fatal(err)
		}
	}
	u, err := s.Collect(t.Context())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// 12,000 bytes is over the limit, so it goes down to 9,000.
	if u.Files != 9 || u.Size != 9000 || u.Removed != 3 {
		t.Errorf("Collect = %+v, want 9 files of 9000 bytes and 3 removed", u)
	}
	for i, k := range keys {
		if _, ok := s.Get("blob", k); ok != (i >= 3) {
			t.Errorf("key %d present = %v, want %v", i, ok, i >= 3)
		}
	}

	// Under the limit, nothing goes.
	u, err = s.Collect(t.Context())
	if err != nil || u.Removed != 0 || u.Files != 9 {
		t.Errorf("second Collect = %+v, %v; want nothing removed", u, err)
	}
}

func TestCollectCountsCompressedSize(t *testing.T) {
	s := open(t, WithMaxSize(int64(len(text))))
	for _, k := range []string{key, strings.Repeat("1", 40)} {
		if err := s.Put("blob", k, text); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	u, err := s.Collect(t.Context())
	if err != nil || u.Removed != 0 || u.Files != 2 || u.Size >= int64(len(text)) {
		t.Errorf("Collect = %+v, %v; want both compressed objects to fit", u, err)
	}
}

func TestCollectRemovesAbandonedFiles(t *testing.T) {
	s := open(t)
	dir := filepath.Join(s.Dir(), "blob", "0f")
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		t.Fatal(err)
	}
	oldTmp, newTmp := filepath.Join(dir, tmpPrefix+"1"), filepath.Join(dir, tmpPrefix+"2")
	for _, p := range []string{oldTmp, newTmp} {
		if err := os.WriteFile(p, text, filePerm); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * tmpMaxAge)
	if err := os.Chtimes(oldTmp, old, old); err != nil {
		t.Fatal(err)
	}
	u, err := s.Collect(t.Context())
	if err != nil || u.Removed != 1 || u.Files != 0 {
		t.Errorf("Collect = %+v, %v; want the old temporary file removed", u, err)
	}
	if exists(t, oldTmp) || !exists(t, newTmp) {
		t.Error("want the old temporary file removed and the recent one, still being written, kept")
	}
}

func TestCollectCanceled(t *testing.T) {
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Collect(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Collect error = %v, want context.Canceled", err)
	}
}

func TestGetCounts(t *testing.T) {
	stats := obs.NewStats()
	prev := obs.SetDefault(stats)
	t.Cleanup(func() { obs.SetDefault(prev) })
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("blob", "ab12", []byte("x")); err != nil {
		t.Fatal(err)
	}
	s.Get("blob", "ab12")
	s.Get("blob", "cd34")
	// Peeks are the revalidator's, not reads.
	s.Peek("blob", "ab12")
	got := stats.Summary().Disk
	if len(got) != 1 || got[0] != (obs.DiskSummary{Kind: "blob", Hit: 1, Miss: 1, HitRatio: 0.5}) {
		t.Errorf("disk = %+v, want a hit and a miss", got)
	}
}
