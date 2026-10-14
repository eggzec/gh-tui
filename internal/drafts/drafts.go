// Package drafts keeps the text of comments that were started and not sent,
// each under a key the caller makes (such as the host, account, repository,
// pull request and target), so the same composer finds it again after a
// restart.
//
// Each draft is a small JSON file of its own in a directory, named by a hash
// of its key, with a version so that a later version can change its shape.
// A file that is missing, damaged or of another version reads as no draft,
// so the worst a bad file does is lose a draft. The text may be about a
// private repository, so the directory and files are for the user alone.
package drafts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Version is the version of the files that this package reads and writes.
const Version = 1

// dirMode and fileMode are those of the directory the store creates and the
// files in it.
const (
	dirMode  = 0o700
	fileMode = 0o600
)

// tmpPrefix starts the names of files being written, as the disk cache
// names its own, so its collection removes any a crash left behind.
const tmpPrefix = ".tmp-"

// ext ends the name of a draft's file.
const ext = ".json"

// file is the shape of a draft's file. The key is kept to tell a draft from
// another whose key hashes to the same name.
type file struct {
	Version int    `json:"version"`
	Key     string `json:"key"`
	Text    string `json:"text"`
}

// Store is the drafts in a directory. Writes replace a draft's file whole,
// so another process never reads half of one; the last to write wins.
type Store struct {
	dir string
}

// New returns the store of the drafts in dir. It touches nothing on disk
// until a draft is saved.
func New(dir string) *Store { return &Store{dir: dir} }

// Dir returns the directory of the drafts.
func (s *Store) Dir() string { return s.dir }

// path returns the path of the file of the draft under key.
func (s *Store) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:16])+ext)
}

// Load returns the draft under key, or "" if there is none. A damaged file,
// or one of another version or of another key, is no draft either, and the
// error says why.
func (s *Store) Load(key string) (string, error) {
	p := s.path(key)
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read draft: %w", err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return "", fmt.Errorf("read draft %s: %w", p, err)
	}
	if f.Version != Version {
		return "", fmt.Errorf("read draft %s: version %d, want %d", p, f.Version, Version)
	}
	if f.Key != key {
		return "", fmt.Errorf("read draft %s: it is another one's", p)
	}
	return f.Text, nil
}

// Save keeps text as the draft under key, replacing the one there. Text of
// white space only is no draft: it deletes any. It creates the directory if
// needed.
func (s *Store) Save(key, text string) error {
	if strings.TrimSpace(text) == "" {
		return s.Delete(key)
	}
	data, err := json.Marshal(file{Version: Version, Key: key, Text: text})
	if err != nil {
		return fmt.Errorf("save draft: %w", err)
	}
	if err := s.write(s.path(key), data); err != nil {
		return fmt.Errorf("save draft: %w", err)
	}
	return nil
}

// Delete removes the draft under key. There being none is no error.
func (s *Store) Delete(key string) error {
	if err := os.Remove(s.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete draft: %w", err)
	}
	return nil
}

// write writes data to a temporary file and renames it to p.
func (s *Store) write(p string, data []byte) error {
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, tmpPrefix+"draft-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// CreateTemp makes the file for the user alone; say so again where the
	// umask or the platform might have it otherwise.
	err = tmp.Chmod(fileMode)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, p)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}
