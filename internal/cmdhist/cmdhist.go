// Package cmdhist keeps the lines typed on the command line between
// sessions, in a small file of their own: JSON with a version, so that a
// later version can change its shape, and the lines, oldest first.
//
// A file that is missing, damaged or of another version reads as empty,
// so the worst a bad file does is forget the history.
package cmdhist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/eggzec/gh-tui/internal/config"
)

// Version is the version of the file that this package reads and writes.
const Version = 1

// FileName is the name of the file in its directory.
const FileName = "cmdline-history.json"

// dirMode is that of the directory the store creates: the lines may name
// private repositories, so only the user may read them.
const dirMode = 0o700

// tmpPrefix starts the names of files being written, as the disk cache
// names its own, so its collection removes any a crash left behind.
const tmpPrefix = ".tmp-"

// file is the shape of the file.
type file struct {
	Version int      `json:"version"`
	Lines   []string `json:"lines"`
}

// Store is the history file at a path. It is safe for concurrent use, and
// writes replace the file whole, so another process never reads half of
// it; the last to write wins.
type Store struct {
	path  string
	limit int
	mu    sync.Mutex
}

// New returns the store of the file at path, which keeps the last limit
// lines, or commands.history of the default config (config.Default) if
// limit isn't positive. It touches nothing on disk until Load or Save.
func New(path string, limit int) *Store {
	if limit <= 0 {
		limit = config.Default().Commands.History
	}
	return &Store{path: path, limit: limit}
}

// Path returns the path of the file.
func (s *Store) Path() string { return s.path }

// Load returns the lines, oldest first, and at most the limit. A missing
// file has none. A damaged file, or one of another version, has none
// either, and the error says why.
func (s *Store) Load() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read command history: %w", err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("read command history %s: %w", s.path, err)
	}
	if f.Version != Version {
		return nil, fmt.Errorf("read command history %s: version %d, want %d", s.path, f.Version, Version)
	}
	return s.trim(f.Lines), nil
}

// Save replaces the lines with lines, oldest first, keeping the last of
// them up to the limit. It creates the directory if needed.
func (s *Store) Save(lines []string) error {
	data, err := json.Marshal(file{Version: Version, Lines: s.trim(lines)})
	if err != nil {
		return fmt.Errorf("save command history: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.write(data); err != nil {
		return fmt.Errorf("save command history: %w", err)
	}
	return nil
}

// write writes data to a temporary file and renames it into place.
func (s *Store) write(data []byte) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	// CreateTemp makes the file for the user alone.
	tmp, err := os.CreateTemp(dir, tmpPrefix+"cmdhist-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, s.path)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}

// trim returns the last lines of lines up to the limit.
func (s *Store) trim(lines []string) []string {
	if len(lines) > s.limit {
		lines = lines[len(lines)-s.limit:]
	}
	return lines
}
