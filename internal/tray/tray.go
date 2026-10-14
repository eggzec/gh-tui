// Package tray keeps the review comments that a user drafted on a pull
// request and has not sent, so that they are there when it is opened again.
//
// Each pull request has a small JSON file of its own in a directory, named by
// a hash of its key, with a version so that a later version can change its
// shape. A file that is missing, damaged or of another version reads as no
// drafts, so the worst a bad file does is lose them. The text may be about a
// private repository, so the directory and files are for the user alone.
package tray

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Version is the version of the files that this package reads and writes.
const Version = 1

// dirMode and fileMode are those of the directory the store creates and the
// files in it.
const (
	dirMode  = 0o700
	fileMode = 0o600
)

// tmpPrefix starts the names of files being written, as the disk cache names
// its own, so its collection removes any a crash left behind.
const tmpPrefix = ".tmp-"

// ext ends the name of a file of drafts.
const ext = ".json"

// Sides of the diff a draft is on, as GitHub names them.
const (
	Left  = "LEFT"
	Right = "RIGHT"
)

// Draft is a review comment that was written and not sent: on a line of a
// file, or on a range of lines of one side of it, or a reply to a thread.
type Draft struct {
	// ID tells the draft from the others of its pull request.
	ID string `json:"id"`
	// Commit is the head of the pull request that the draft was written on,
	// which says later whether the line it is on has moved.
	Commit string `json:"commit"`
	Path   string `json:"path"`
	// Side is Left for a deleted line and Right otherwise.
	Side string `json:"side"`
	// Line is the line the comment is on, or the last of its range, and
	// StartLine the first of a range, or 0 for a single line.
	Line      int `json:"line"`
	StartLine int `json:"start_line,omitempty"`
	// ThreadID is the thread that a draft replies to, which is then not on a
	// line of its own.
	ThreadID string    `json:"thread_id,omitempty"`
	Body     string    `json:"body"`
	Created  time.Time `json:"created"`
}

// file is the shape of the file of the drafts of a pull request. The key is
// kept to tell a pull request from another whose key hashes to the same name.
type file struct {
	Version int     `json:"version"`
	Key     string  `json:"key"`
	Drafts  []Draft `json:"drafts"`
}

// ErrDamaged is what a read error wraps when the file was read but is not
// drafts this version can use: not JSON, of another version, or of another
// key. Saving may replace such a file; any other read error, such as a denied
// or failed read, says nothing of the file, and saving over it could lose
// drafts.
var ErrDamaged = errors.New("damaged")

// Store is the drafts in a directory. Writes replace a file whole, so another
// process never reads half of one; the last to write wins. Calls of one store
// are serialized, so a delete never lands before a save that was made earlier.
type Store struct {
	dir string
	mu  sync.Mutex
}

// New returns the store of the drafts in dir. It touches nothing on disk
// until drafts are saved.
func New(dir string) *Store { return &Store{dir: dir} }

// Dir returns the directory of the drafts.
func (s *Store) Dir() string { return s.dir }

// Key returns the key of the drafts of pull request number of repo, a name
// such as "owner/name".
func Key(repo string, number int) string { return fmt.Sprintf("%s#%d", repo, number) }

// path returns the path of the file under key.
func (s *Store) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:16])+ext)
}

// Load returns the drafts under key, or none. A damaged file, or one of
// another version or of another key, is no drafts either, and the error says
// why, wrapping [ErrDamaged].
func (s *Store) Load(key string) ([]Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.path(key)
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read drafts: %w", err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("read drafts %s: %w: %w", p, ErrDamaged, err)
	}
	if f.Version != Version {
		return nil, fmt.Errorf("read drafts %s: %w: version %d, want %d", p, ErrDamaged, f.Version, Version)
	}
	if f.Key != key {
		return nil, fmt.Errorf("read drafts %s: %w: they are another pull request's", p, ErrDamaged)
	}
	return f.Drafts, nil
}

// Save keeps drafts as those under key, replacing what is there. No drafts
// delete the file. It creates the directory if needed.
func (s *Store) Save(key string, drafts []Draft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(drafts) == 0 {
		if err := os.Remove(s.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("delete drafts: %w", err)
		}
		return nil
	}
	data, err := json.Marshal(file{Version: Version, Key: key, Drafts: drafts})
	if err != nil {
		return fmt.Errorf("save drafts: %w", err)
	}
	if err := s.write(s.path(key), data); err != nil {
		return fmt.Errorf("save drafts: %w", err)
	}
	return nil
}

// write writes data to a temporary file and renames it to p.
func (s *Store) write(p string, data []byte) error {
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, tmpPrefix+"tray-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// Say again that the file is for the user alone, where the umask or the
	// platform might have it otherwise.
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
