package tray

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// drafts is what Store and Memory share, so that one test runs both.
type drafts interface {
	Load(key string) ([]Draft, error)
	Put(key string, d Draft) (Draft, error)
	Remove(key, id string) error
}

func eachKeeper(t *testing.T, f func(t *testing.T, k drafts)) {
	t.Helper()
	t.Run("store", func(t *testing.T) {
		t.Parallel()
		f(t, New(filepath.Join(t.TempDir(), "tray")))
	})
	t.Run("memory", func(t *testing.T) {
		t.Parallel()
		f(t, &Memory{})
	})
}

func TestPutAddsAndStamps(t *testing.T) {
	t.Parallel()
	eachKeeper(t, func(t *testing.T, k drafts) {
		t.Helper()
		key := Key("o/r", 1)
		first, err := k.Put(key, Draft{Path: "a.go", Line: 3, Body: "one"})
		if err != nil {
			t.Fatal(err)
		}
		if first.ID == "" || first.Created.IsZero() {
			t.Fatalf("draft = %+v, want an ID and a time", first)
		}
		second, err := k.Put(key, Draft{Path: "b.go", Line: 4, Body: "two", Created: time.Unix(5, 0).UTC()})
		if err != nil {
			t.Fatal(err)
		}
		if second.ID == first.ID || !second.Created.Equal(time.Unix(5, 0)) {
			t.Fatalf("second = %+v, want a new ID and its own time", second)
		}
		got, err := k.Load(key)
		if err != nil || len(got) != 2 || got[0].Body != "one" || got[1].Body != "two" {
			t.Fatalf("Load = %+v, %v; want both, in order", got, err)
		}
		if other, err := k.Load(Key("o/r", 2)); err != nil || len(other) != 0 {
			t.Fatalf("Load of another pull request = %+v, %v", other, err)
		}
	})
}

func TestPutReplacesByID(t *testing.T) {
	t.Parallel()
	eachKeeper(t, func(t *testing.T, k drafts) {
		t.Helper()
		key := Key("o/r", 1)
		a, err := k.Put(key, Draft{ID: "a", Body: "old"})
		if err != nil {
			t.Fatal(err)
		}
		if a.ID != "a" {
			t.Fatalf("ID = %q, want the one given", a.ID)
		}
		if _, err := k.Put(key, Draft{ID: "b", Body: "other"}); err != nil {
			t.Fatal(err)
		}
		if _, err := k.Put(key, Draft{ID: "a", Body: "new"}); err != nil {
			t.Fatal(err)
		}
		got, err := k.Load(key)
		if err != nil || len(got) != 2 || got[0].ID != "a" || got[0].Body != "new" || got[1].Body != "other" {
			t.Fatalf("Load = %+v, %v; want a replaced in place and b kept", got, err)
		}
	})
}

func TestRemove(t *testing.T) {
	t.Parallel()
	eachKeeper(t, func(t *testing.T, k drafts) {
		t.Helper()
		key := Key("o/r", 1)
		if err := k.Remove(key, "none"); err != nil {
			t.Fatalf("Remove with nothing kept = %v", err)
		}
		for _, id := range []string{"a", "b"} {
			if _, err := k.Put(key, Draft{ID: id, Body: id}); err != nil {
				t.Fatal(err)
			}
		}
		if err := k.Remove(key, "a"); err != nil {
			t.Fatal(err)
		}
		if err := k.Remove(key, "missing"); err != nil {
			t.Errorf("Remove of a draft that is not there = %v", err)
		}
		got, err := k.Load(key)
		if err != nil || len(got) != 1 || got[0].ID != "b" {
			t.Fatalf("Load = %+v, %v; want only b", got, err)
		}
		if err := k.Remove(key, "b"); err != nil {
			t.Fatal(err)
		}
		if got, err := k.Load(key); err != nil || len(got) != 0 {
			t.Fatalf("Load after removing all = %+v, %v", got, err)
		}
	})
}

// A damaged file does not stop a new draft, which replaces it, while Remove
// reports it.
func TestPutReplacesADamagedFile(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	key := Key("o/r", 1)
	if err := os.WriteFile(s.path(key), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(key, "a"); !errors.Is(err, ErrDamaged) {
		t.Errorf("Remove on a damaged file = %v, want ErrDamaged", err)
	}
	if _, err := s.Put(key, Draft{ID: "a", Body: "x"}); err != nil {
		t.Fatalf("Put on a damaged file = %v", err)
	}
	if got, err := s.Load(key); err != nil || len(got) != 1 {
		t.Fatalf("Load = %+v, %v; want the new draft", got, err)
	}
}

// Memory hands out copies, so a caller that changes them changes nothing kept.
func TestMemoryLoadIsACopy(t *testing.T) {
	t.Parallel()
	var m Memory
	if _, err := m.Put("k", Draft{ID: "a", Body: "x"}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Load("k")
	got[0].Body = "changed"
	if again, _ := m.Load("k"); again[0].Body != "x" {
		t.Errorf("Body = %q, want the kept one", again[0].Body)
	}
}
