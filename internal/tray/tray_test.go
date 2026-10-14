package tray

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSaveLoad(t *testing.T) {
	t.Parallel()
	s := New(filepath.Join(t.TempDir(), "tray"))
	key := Key("o/r", 4)
	want := []Draft{
		{ID: "a", Commit: "abc", Path: "x.go", Side: Right, Line: 5, StartLine: 3, Body: "range", Text: "cfg := load()", Outdated: true, Created: time.Unix(10, 0).UTC()},
		{ID: "b", Commit: "abc", Path: "x.go", ThreadID: "T1", Body: "reply", Created: time.Unix(20, 0).UTC()},
	}
	got, err := s.Load(key)
	if err != nil || len(got) != 0 {
		t.Fatalf("Load before any save = %v, %v", got, err)
	}

	if err := s.Save(key, want); err != nil {
		t.Fatal(err)
	}
	got, err = s.Load(key)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %v, %v; want %v", got, err, want)
	}

	if other, err := s.Load(Key("o/r", 5)); err != nil || len(other) != 0 {
		t.Fatalf("Load of another pull request = %v, %v", other, err)
	}

	files, err := os.ReadDir(s.Dir())
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %v, %v", files, err)
	}
	info, err := files[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}

	if err := s.Save(key, nil); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Load(key); err != nil || len(got) != 0 {
		t.Fatalf("Load after saving none = %v, %v", got, err)
	}
	if err := s.Save(key, nil); err != nil {
		t.Errorf("deleting none: %v", err)
	}
}

// A draft that is not outdated, or has no text, writes neither, so that a file
// of drafts from before them reads the same as one written now.
func TestOutdatedAndTextAreOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	key := Key("o/r", 7)
	if err := s.Save(key, []Draft{{ID: "a", Body: "x"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path(key))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "outdated") || strings.Contains(string(data), `"text"`) {
		t.Errorf("file = %s, want neither outdated nor text", data)
	}
}

// A delete that comes after a save, even when they race, leaves no file.
func TestDeleteAfterSaveLeavesNoFile(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	key := Key("o/r", 3)
	for range 50 {
		if err := s.Save(key, []Draft{{ID: "a", Body: "x"}}); err != nil {
			t.Fatal(err)
		}
		if err := s.Save(key, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(s.path(key)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("stat after the delete = %v, want no file", err)
		}
	}
	// Calls from several goroutines are safe.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			if i%2 == 0 {
				_ = s.Save(key, []Draft{{ID: "a"}})
			} else {
				_, _ = s.Load(key)
			}
		})
	}
	wg.Wait()
}

// A file that could not be read for a reason other than its content is not
// said to be damaged, so that nothing writes over it.
func TestLoadDamagedOnlyForContent(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	key := Key("o/r", 1)
	if err := os.WriteFile(s.path(key), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(key); !errors.Is(err, ErrDamaged) {
		t.Errorf("a file that is not JSON: %v, want ErrDamaged", err)
	}
	// A directory where the file should be reads with an error of its own.
	key = Key("o/r", 2)
	if err := os.Mkdir(s.path(key), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(key); err == nil || errors.Is(err, ErrDamaged) {
		t.Errorf("an unreadable file: %v, want an error that is not ErrDamaged", err)
	}
}

func TestLoadRefusesBadFiles(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	key := Key("o/r", 1)
	for name, data := range map[string]string{
		"damaged": "{",
		"version": `{"version":99,"key":"o/r#1","drafts":[{"id":"a"}]}`,
		"key":     `{"version":1,"key":"o/r#2","drafts":[{"id":"a"}]}`,
	} {
		if err := os.WriteFile(s.path(key), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := s.Load(key)
		if err == nil || len(got) != 0 {
			t.Errorf("%s: Load = %v, %v; want an error and none", name, got, err)
		}
	}
}
