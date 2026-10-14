package drafts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLoadDelete(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "drafts"))
	if got, err := s.Load("a"); got != "" || err != nil {
		t.Fatalf("Load with no file = %q, %v", got, err)
	}
	if err := s.Save("a", "text of a\nline 2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("b", "text of b"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load("a"); got != "text of a\nline 2" || err != nil {
		t.Errorf("Load(a) = %q, %v", got, err)
	}
	if got, _ := s.Load("b"); got != "text of b" {
		t.Errorf("Load(b) = %q", got)
	}
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load("a"); got != "" {
		t.Errorf("Load(a) after Delete = %q", got)
	}
	if err := s.Delete("a"); err != nil {
		t.Errorf("deleting what isn't there: %v", err)
	}
	// Text of white space only is no draft.
	if err := s.Save("b", " \n "); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load("b"); got != "" {
		t.Errorf("Load(b) after blank save = %q", got)
	}
}

func TestFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix modes")
	}
	s := New(filepath.Join(t.TempDir(), "drafts"))
	if err := s.Save("a", "x"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(s.path("a"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", fi.Mode().Perm())
	}
	di, _ := os.Stat(s.Dir())
	if di.Mode().Perm() != 0o700 {
		t.Errorf("directory mode %v, want 0700", di.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(s.Dir(), tmpPrefix+"*")); len(left) != 0 {
		t.Errorf("left %v behind", left)
	}
}

// A damaged file, one of another version and one of another key read as no
// draft, with the reason, and a save replaces them.
func TestBadFiles(t *testing.T) {
	for name, data := range map[string]string{
		"corrupt":      "{not json",
		"old version":  `{"version":0,"key":"a","text":"old"}`,
		"new version":  `{"version":2,"key":"a","text":"new"}`,
		"other key":    `{"version":1,"key":"other","text":"theirs"}`,
		"empty file":   "",
		"wrong shape":  `["a"]`,
		"no version":   `{"key":"a","text":"x"}`,
		"null version": `{"version":null,"key":"a","text":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := New(t.TempDir())
			if err := os.WriteFile(s.path("a"), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := s.Load("a"); got != "" || err == nil {
				t.Errorf("Load = %q, %v; want no draft and an error", got, err)
			}
			if err := s.Save("a", "fresh"); err != nil {
				t.Fatal(err)
			}
			if got, err := s.Load("a"); got != "fresh" || err != nil {
				t.Errorf("Load after Save = %q, %v", got, err)
			}
		})
	}
}
