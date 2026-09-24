package disk

import (
	"bytes"
	"compress/gzip"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestList(t *testing.T) {
	s := open(t)
	keys := []string{key, "ab01", "ab02", "cd03"}
	for _, k := range keys {
		if err := s.Put("entry", k, text); err != nil {
			t.Fatal(err)
		}
	}
	// A plain copy next to a compressed one is one object.
	if err := os.WriteFile(objectPath(s, "entry", "ab01"), text, filePerm); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("other", "ef04", text); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".tmp-123", "not-hex", "ZZZZ"} {
		if err := os.WriteFile(filepath.Join(s.Dir(), "entry", "ab", name), nil, filePerm); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(objectPath(s, "entry", "cd03")+gzExt, old, old); err != nil {
		t.Fatal(err)
	}

	got := maps.Collect(s.List("entry"))
	if want := slices.Sorted(slices.Values(keys)); !slices.Equal(slices.Sorted(maps.Keys(got)), want) {
		t.Errorf("List = %v, want %v", slices.Sorted(maps.Keys(got)), want)
	}
	if !got["cd03"].Equal(old) {
		t.Errorf("used at %v, want the modification time %v", got["cd03"], old)
	}
	if n := len(maps.Collect(s.List("missing"))); n != 0 {
		t.Errorf("List of a kind never put yields %d objects, want 0", n)
	}
	if n := len(maps.Collect(s.List("../entry"))); n != 0 {
		t.Errorf("List of an invalid kind yields %d objects, want 0", n)
	}
	for range s.List("entry") {
		break // stopping early must not panic
	}
}

func TestPeekDoesNotTouch(t *testing.T) {
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatal(err)
	}
	p := objectPath(s, "blob", key) + gzExt
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if b, ok := s.Peek("blob", key); !ok || !bytes.Equal(b, text) {
		t.Fatalf("Peek = %d bytes, %v; want the object", len(b), ok)
	}
	if fi, err := os.Stat(p); err != nil || !fi.ModTime().Equal(old) {
		t.Errorf("modification time after Peek = %v, want %v unchanged", fi.ModTime(), old)
	}
}

func TestReplaceKeepsUse(t *testing.T) {
	for _, level := range []int{gzip.DefaultCompression, gzip.NoCompression} {
		t.Run(strconv.Itoa(level), func(t *testing.T) {
			s := open(t, WithCompression(level))
			if err := s.Put("entry", key, text); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
			used := maps.Collect(s.List("entry"))
			for _, name := range []string{objectPath(s, "entry", key), objectPath(s, "entry", key) + gzExt} {
				_ = os.Chtimes(name, old, old)
			}
			newer := append([]byte("newer "), text...)
			if err := s.Replace("entry", key, newer); err != nil {
				t.Fatalf("Replace: %v", err)
			}
			if b, ok := s.Peek("entry", key); !ok || !bytes.Equal(b, newer) {
				t.Errorf("Peek after Replace = %q, want the new content", b)
			}
			if got := maps.Collect(s.List("entry"))[key]; !got.Equal(old) {
				t.Errorf("used at %v after Replace, want %v as before (was %v when put)", got, old, used[key])
			}
			if err := s.Replace("entry", "ab01", text); err != nil {
				t.Fatalf("Replace of a new object: %v", err)
			}
			if got := maps.Collect(s.List("entry"))["ab01"]; time.Since(got) > time.Minute {
				t.Errorf("new object used at %v, want now", got)
			}
			if err := s.Replace("entry", "x", text); err == nil {
				t.Error("Replace of an invalid name succeeded")
			}
		})
	}
}
