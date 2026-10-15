package visits

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmptyIgnoresTheLocationOfTheTime(t *testing.T) {
	t.Parallel()
	// A zero instant in another location differs from the zero value under ==.
	other := State{LastSeen: LastSeen{At: time.Time{}.In(time.FixedZone("x", 3600))}}
	if !other.Empty() {
		t.Errorf("State with a zero time in a zone is not Empty")
	}
	for name, s := range map[string]State{
		"head":   {LastSeen: LastSeen{Head: "a"}},
		"at":     {LastSeen: LastSeen{At: time.Unix(1, 0)}},
		"viewed": {Viewed: map[string]string{"a": "b"}},
	} {
		if s.Empty() {
			t.Errorf("State with only %s is Empty", name)
		}
	}
}

func TestSaveLoad(t *testing.T) {
	t.Parallel()
	s := New(filepath.Join(t.TempDir(), "visits"))
	key := Key("o/r", 4)
	want := State{
		LastSeen: LastSeen{Head: "abc", At: time.Unix(10, 0).UTC()},
		Viewed:   map[string]string{"a.go": "b1", "dir/b.go": "b2"},
	}
	got, err := s.Load(key)
	if err != nil || !got.Empty() {
		t.Fatalf("Load before any save = %v, %v", got, err)
	}

	if err := s.Save(key, want); err != nil {
		t.Fatal(err)
	}
	got, err = s.Load(key)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %v, %v; want %v", got, err, want)
	}

	if other, err := s.Load(Key("o/r", 5)); err != nil || !other.Empty() {
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
	dir, err := os.Stat(s.Dir())
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Errorf("directory = %v, %v, want 0700", dir, err)
	}

	if err := s.Save(key, State{}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.Load(key); err != nil || !got.Empty() {
		t.Fatalf("Load after saving an empty state = %v, %v", got, err)
	}
	if _, err := os.Stat(s.path(key)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat after saving an empty state = %v, want no file", err)
	}
	if err := s.Save(key, State{}); err != nil {
		t.Errorf("deleting none: %v", err)
	}
}

// Either part of a state is kept without the other.
func TestSaveKeepsEitherPart(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	for name, want := range map[string]State{
		"seen":   {LastSeen: LastSeen{Head: "abc", At: time.Unix(1, 0).UTC()}},
		"viewed": {Viewed: map[string]string{"a.go": "b1"}},
	} {
		key := Key("o/"+name, 1)
		if err := s.Save(key, want); err != nil {
			t.Fatal(err)
		}
		got, err := s.Load(key)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Load = %v, %v; want %v", name, got, err, want)
		}
	}
}

// A state with only viewed files is written without the empty last_seen, and
// a file that has one, as earlier versions wrote it, still decodes.
func TestLastSeenIsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	key := Key("o/r", 7)
	want := State{Viewed: map[string]string{"a.go": "b1"}}
	if err := s.Save(key, want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path(key))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "last_seen") {
		t.Errorf("file = %s, want no last_seen", data)
	}

	old := `{"version":` + strconv.Itoa(Version) + `,"key":"` + key + `","last_seen":{},"viewed":{"a.go":"b1"}}`
	if err := os.WriteFile(s.path(key), []byte(old), fileMode); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(key)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Load of a file with an empty last_seen = %v, %v; want %v", got, err, want)
	}
}

// A delete that comes after a save, even when they race, leaves no file.
func TestDeleteAfterSaveLeavesNoFile(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	key := Key("o/r", 3)
	state := State{Viewed: map[string]string{"a.go": "b1"}}
	for range 50 {
		if err := s.Save(key, state); err != nil {
			t.Fatal(err)
		}
		if err := s.Save(key, State{}); err != nil {
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
				_ = s.Save(key, state)
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
		"version": `{"version":99,"key":"o/r#1","viewed":{"a.go":"b1"}}`,
		"key":     `{"version":1,"key":"o/r#2","viewed":{"a.go":"b1"}}`,
	} {
		if err := os.WriteFile(s.path(key), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := s.Load(key)
		if !errors.Is(err, ErrDamaged) || !got.Empty() {
			t.Errorf("%s: Load = %v, %v; want ErrDamaged and an empty state", name, got, err)
		}
	}
}
