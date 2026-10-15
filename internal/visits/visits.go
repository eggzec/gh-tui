// Package visits keeps, for each pull request, when the user last looked at it
// and which of its files they marked as viewed, so that it can show what is new
// since and tick the files already seen.
//
// Each pull request has a small JSON file of its own in a directory, named by
// a hash of its key, with a version so that a later version can change its
// shape. A file that is missing, damaged or of another version reads as no
// visit, so the worst a bad file does is lose them. The names may be of a
// private repository, so the directory and files are for the user alone.
package visits

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

// ext ends the name of a file of a pull request's visits.
const ext = ".json"

// LastSeen is the last time the user looked at a pull request.
type LastSeen struct {
	// Head is the commit that was the head of the pull request then, which
	// says later what was pushed since.
	Head string    `json:"head,omitempty"`
	At   time.Time `json:"at,omitzero"`
}

// State is what is kept of a pull request.
type State struct {
	LastSeen LastSeen `json:"last_seen,omitzero"`
	// Viewed maps the path of each file the user marked as viewed to the SHA of
	// its blob then, so that a file that changed since is not taken for seen.
	Viewed map[string]string `json:"viewed,omitempty"`
}

// Empty reports whether the state says nothing, which is kept as no file. The
// time is compared with IsZero, since a time decoded from a file may be the
// zero instant and still differ from the zero value under ==.
func (s State) Empty() bool {
	return s.LastSeen.Head == "" && s.LastSeen.At.IsZero() && len(s.Viewed) == 0
}

// file is the shape of the file of a pull request. The key is kept to tell a
// pull request from another whose key hashes to the same name.
type file struct {
	Version int    `json:"version"`
	Key     string `json:"key"`
	State
}

// ErrDamaged is what a read error wraps when the file was read but is not a
// state this version can use: not JSON, of another version, or of another key.
// Saving may replace such a file; any other read error, such as a denied or
// failed read, says nothing of the file, and saving over it could lose what
// it holds.
var ErrDamaged = errors.New("damaged")

// Store is the visits in a directory. Writes replace a file whole, so another
// process never reads half of one; the last to write wins. Calls of one store
// are serialized, so a delete never lands before a save that was made earlier.
type Store struct {
	dir string
	mu  sync.Mutex
}

// New returns the store of the visits in dir. It touches nothing on disk until
// a state is saved.
func New(dir string) *Store { return &Store{dir: dir} }

// Dir returns the directory of the visits.
func (s *Store) Dir() string { return s.dir }

// Key returns the key of the visits to pull request number of repo, a name
// such as "owner/name".
func Key(repo string, number int) string { return fmt.Sprintf("%s#%d", repo, number) }

// path returns the path of the file under key.
func (s *Store) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:16])+ext)
}

// Load returns the state under key, or an empty one. A damaged file, or one of
// another version or of another key, is an empty state too, and the error says
// why, wrapping [ErrDamaged].
func (s *Store) Load(key string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.path(key)
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read visits: %w", err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return State{}, fmt.Errorf("read visits %s: %w: %w", p, ErrDamaged, err)
	}
	if f.Version != Version {
		return State{}, fmt.Errorf("read visits %s: %w: version %d, want %d", p, ErrDamaged, f.Version, Version)
	}
	if f.Key != key {
		return State{}, fmt.Errorf("read visits %s: %w: they are another pull request's", p, ErrDamaged)
	}
	return f.State, nil
}

// Save keeps state as that under key, replacing what is there. An empty state
// deletes the file. It creates the directory if needed. Save does not keep
// state: it is encoded before Save returns. The caller must not change the
// Viewed map from another goroutine while Save runs, so a caller that shares
// the map passes a copy.
func (s *Store) Save(key string, state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.Empty() {
		if err := os.Remove(s.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("delete visits: %w", err)
		}
		return nil
	}
	data, err := json.Marshal(file{Version: Version, Key: key, State: state})
	if err != nil {
		return fmt.Errorf("save visits: %w", err)
	}
	if err := s.write(s.path(key), data); err != nil {
		return fmt.Errorf("save visits: %w", err)
	}
	return nil
}

// write writes data to a temporary file and renames it to p.
func (s *Store) write(p string, data []byte) error {
	if err := os.MkdirAll(s.dir, dirMode); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, tmpPrefix+"visits-*")
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
