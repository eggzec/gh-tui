package cmdhist

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", FileName)
	s := New(path, 0)
	lines := []string{"goto cli/cli", "goto #12", "q"}
	if err := s.Save(lines); err != nil {
		t.Fatal(err)
	}
	got, err := New(path, 0).Load()
	if err != nil || !slices.Equal(got, lines) {
		t.Fatalf("Load = %q, %v, want %q", got, err, lines)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"version":1,"lines":["goto cli/cli","goto #12","q"]}`; string(data) != want {
		t.Errorf("file = %s, want %s", data, want)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v, %v, want 0600", fi.Mode().Perm(), err)
		}
		if fi, err := os.Stat(filepath.Dir(path)); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("directory mode = %v, %v, want 0700", fi.Mode().Perm(), err)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("directory holds %d files, want the history alone", len(entries))
	}
}

func TestLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	lines := make([]string, 0, 150)
	for i := range 150 {
		lines = append(lines, fmt.Sprintf("goto #%d", i))
	}
	if err := New(path, 0).Save(lines); err != nil {
		t.Fatal(err)
	}
	got, err := New(path, 0).Load()
	if err != nil || len(got) != DefaultLimit || got[0] != "goto #50" || got[99] != "goto #149" {
		t.Errorf("Load = %d lines from %q, %v, want the last %d", len(got), got[0], err, DefaultLimit)
	}
	// A smaller limit reads a larger file's last lines.
	got, err = New(path, 3).Load()
	if err != nil || !slices.Equal(got, lines[147:]) {
		t.Errorf("Load with limit 3 = %q, %v, want %q", got, err, lines[147:])
	}
}

func TestMissingIsEmpty(t *testing.T) {
	got, err := New(filepath.Join(t.TempDir(), "none", FileName), 0).Load()
	if err != nil || got != nil {
		t.Errorf("Load = %q, %v, want nothing and no error", got, err)
	}
}

func TestBadFileIsEmpty(t *testing.T) {
	for name, content := range map[string]string{
		"corrupt":       `{"version":1,"lines":["goto`,
		"not json":      "\x00\x01",
		"other version": `{"version":2,"lines":["goto cli/cli"]}`,
		"no version":    `{"lines":["goto cli/cli"]}`,
		"wrong shape":   `{"version":1,"lines":"goto cli/cli"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			s := New(path, 0)
			got, err := s.Load()
			if err == nil || got != nil {
				t.Fatalf("Load = %q, %v, want nothing and an error", got, err)
			}
			// Saving replaces the bad file.
			if err := s.Save([]string{"q"}); err != nil {
				t.Fatal(err)
			}
			if got, err := s.Load(); err != nil || !slices.Equal(got, []string{"q"}) {
				t.Errorf("Load after Save = %q, %v", got, err)
			}
		})
	}
}

func TestSaveFails(t *testing.T) {
	// A file where the directory should be.
	dir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := New(filepath.Join(dir, FileName), 0).Save([]string{"q"})
	if err == nil || !strings.Contains(err.Error(), "save command history") {
		t.Errorf("Save = %v, want an error", err)
	}
}

func TestConcurrentSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := New(path, 0)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			if err := s.Save([]string{strconv.Itoa(i)}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got, err := s.Load(); err != nil || len(got) != 1 {
		t.Errorf("Load = %q, %v, want one whole save", got, err)
	}
}
