package logfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func open(t *testing.T, path string, maxSize int64, keep int) *File {
	t.Helper()
	l, err := Open(path, maxSize, keep)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

// line returns a JSON line numbered i. Lines are all of one length, so
// that tests can tell how many fit in a file.
func line(i int) []byte {
	n := strconv.Itoa(i)
	return fmt.Appendf(nil, `{"n":%s,"pad":"%s"}`+"\n", n, strings.Repeat("x", 40-len(n)))
}

func TestOpenCreatesPrivateFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix permissions")
	}
	path := filepath.Join(t.TempDir(), "state", "gh-tui", "gh-tui.log")
	l := open(t, path, 1<<20, 3)
	if _, err := l.Write([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dir.Mode().Perm(); got != 0o700 {
		t.Errorf("dir mode = %v, want 0700", got)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %v, want 0600", got)
	}
}

func TestAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := open(t, path, 1<<20, 3)
	if _, err := l.Write([]byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old\nnew\n" {
		t.Errorf("file = %q, want the old line kept", got)
	}
}

func TestRotateKeeps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	size := int64(len(line(0)))
	// Three lines fit in a file.
	l := open(t, path, 3*size, 2)
	for i := range 20 {
		if _, err := l.Write(line(i)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"a.log", "a.log.1", "a.log.2"}; fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("files = %v, want %v", names, want)
	}
	for _, name := range names {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Size() > 3*size {
			t.Errorf("%s has %d bytes, want at most %d", name, fi.Size(), 3*size)
		}
	}
	// 20 lines, 3 a file: the current file has the last two.
	if got, _ := os.ReadFile(path); !bytes.Equal(got, append(line(18), line(19)...)) {
		t.Errorf("current file = %q, want lines 18 and 19", got)
	}
	if got, _ := os.ReadFile(path + ".1"); !bytes.HasPrefix(got, line(15)) {
		t.Errorf("a.log.1 = %q, want lines 15 to 17", got)
	}
}

func TestRotateKeepNone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	l := open(t, path, int64(len(line(0))), 0)
	for i := range 5 {
		if _, err := l.Write(line(i)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("got %d files, want only the current one", len(entries))
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, line(4)) {
		t.Errorf("file = %q, want the last line", got)
	}
}

// TestConcurrentAppenders writes from several Files on one path, as
// several processes would, and checks that every line arrives whole.
func TestConcurrentAppenders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	const (
		writers = 4
		lines   = 400
	)
	var wg sync.WaitGroup
	for w := range writers {
		l := open(t, path, 4<<10, 1000)
		wg.Go(func() {
			for i := range lines {
				if _, err := l.Write(line(w*lines + i)); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()

	files, err := filepath.Glob(path + "*")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[int]bool)
	for _, name := range files {
		if strings.HasSuffix(name, ".lock") {
			t.Errorf("lock file %s left behind", name)
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for l := range strings.Lines(string(data)) {
			var rec struct{ N int }
			if err := json.Unmarshal([]byte(l), &rec); err != nil {
				t.Fatalf("%s has a broken line %q: %v", filepath.Base(name), l, err)
			}
			if seen[rec.N] {
				t.Errorf("line %d written twice", rec.N)
			}
			seen[rec.N] = true
		}
	}
	if len(seen) != writers*lines {
		t.Errorf("got %d lines, want %d", len(seen), writers*lines)
	}
	if len(files) < 2 {
		t.Errorf("got %d files, want the log rotated", len(files))
	}
}

func TestFollowsRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	a := open(t, path, 1<<20, 3)
	if _, err := a.Write(line(0)); err != nil {
		t.Fatal(err)
	}
	// Another process rotated the file.
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.followed = time.Time{}
	a.mu.Unlock()
	if _, err := a.Write(line(1)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, line(1)) {
		t.Errorf("new file = %q, want the line written after the rotation", got)
	}
}

func TestLockedRotationFollows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	size := int64(len(line(0)))
	l := open(t, path, size, 3)
	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err := l.Write(line(i)); err != nil {
			t.Fatal(err)
		}
	}
	// Another process holds the lock, so this one doesn't rotate.
	if _, err := os.Stat(path + ".1"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("rotated while another process held the lock: %v", err)
	}

	// A lock that old is stale.
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path+".lock", old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write(line(3)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("didn't rotate past a stale lock: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale lock left behind: %v", err)
	}
}

func TestWriteAfterClose(t *testing.T) {
	l := open(t, filepath.Join(t.TempDir(), "a.log"), 1<<20, 3)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write(line(0)); !errors.Is(err, fs.ErrClosed) {
		t.Errorf("Write after Close = %v, want fs.ErrClosed", err)
	}
}

func BenchmarkWrite(b *testing.B) {
	l, err := Open(filepath.Join(b.TempDir(), "a.log"), 1<<20, 2)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()
	p := line(0)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := l.Write(p); err != nil {
			b.Fatal(err)
		}
	}
}
