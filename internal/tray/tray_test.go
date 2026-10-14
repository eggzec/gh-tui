package tray

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSaveLoad(t *testing.T) {
	t.Parallel()
	s := New(filepath.Join(t.TempDir(), "tray"))
	key := Key("o/r", 4)
	want := []Draft{
		{ID: "a", Commit: "abc", Path: "x.go", Side: Right, Line: 5, StartLine: 3, Body: "range", Created: time.Unix(10, 0).UTC()},
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
