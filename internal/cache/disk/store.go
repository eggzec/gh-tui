// Package disk is the optional disk layer of the cache: a directory of
// objects that outlives the process, so a new session starts from what the
// last one read.
//
// Objects are named by a kind, such as "blob", and a hex key, such as a
// content hash. Each lives in its own file, <dir>/<kind>/<key[:2]>/<key>,
// with a .gz suffix when it is compressed, so the files stay readable with
// zcat. Writes go to a temporary file that is renamed into place, so readers
// and other processes never see half an object. A file that fails to
// decompress reads as a miss and is removed.
//
// The store keeps no state of its own besides its options, so any number of
// processes may share a directory.
package disk

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// gzExt marks a compressed object.
const gzExt = ".gz"

// Permissions of what the store creates. Objects may hold the content of
// private repositories, so only the user may read them.
const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// touchAfter is how old an object's modification time must be before a read
// updates it. Eviction goes by that time, so objects in use stay, while a
// session reading the same object again doesn't write each time.
const touchAfter = time.Hour

// logEvery is how often a failure is logged for each kind of object and
// operation. A full disk fails every write, and one record says so.
const logEvery = 10 * time.Minute

// Store is a directory of objects. It is safe for concurrent use, also by
// several processes. Create one with Open.
type Store struct {
	dir  string
	opts options
	// writers holds *gzip.Writer of opts.level for reuse: a new one
	// allocates about a megabyte of tables.
	writers sync.Pool
	// logs throttles the records of failures.
	logs *obs.Throttle
}

// Open returns the store in dir, creating the directory if needed.
func Open(dir string, opts ...Option) (*Store, error) {
	o := options{maxSize: DefaultMaxSize, level: gzip.DefaultCompression}
	for _, opt := range opts {
		opt(&o)
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("open disk cache: %w", err)
	}
	return &Store{dir: dir, opts: o, logs: obs.NewThrottle(logEvery)}, nil
}

// Dir returns the directory of the store.
func (s *Store) Dir() string {
	return s.dir
}

// Get returns the object of kind named key, and false if there is none or
// it can't be read. Reading an object counts as using it, for Collect.
func (s *Store) Get(kind, key string) ([]byte, bool) {
	b, ok := s.get(kind, key, true)
	if ok {
		obs.CountCache(kind, obs.DiskHit)
	} else {
		obs.CountCache(kind, obs.DiskMiss)
	}
	return b, ok
}

// Peek is Get without counting as a use, for reading objects in the
// background, such as to revalidate them, without keeping them from being
// collected.
func (s *Store) Peek(kind, key string) ([]byte, bool) {
	return s.get(kind, key, false)
}

func (s *Store) get(kind, key string, use bool) ([]byte, bool) {
	p, ok := s.path(kind, key)
	if !ok {
		return nil, false
	}
	// The format written now is likelier, but objects written with another
	// setting stay readable.
	first, second := p+gzExt, p
	if !s.compressed() {
		first, second = second, first
	}
	for _, name := range []string{first, second} {
		b, err := read(name, use)
		if err == nil {
			return b, true
		}
		if !errors.Is(err, fs.ErrNotExist) {
			// A corrupt object is worth nothing; the next Put replaces it.
			s.dropped(kind, err)
			if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				s.failed("delete", kind, err)
			}
		}
	}
	return nil, false
}

// read returns the content of the object in name, decompressing it if its
// name says so. If use is set, it updates the modification time of an old
// object.
func read(name string, use bool) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	b := make([]byte, fi.Size())
	_, err = io.ReadFull(f, b)
	if err == nil && filepath.Ext(name) == gzExt {
		b, err = gunzip(b)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if now := time.Now(); use && now.Sub(fi.ModTime()) > touchAfter {
		// Only eviction depends on it, so a failure doesn't matter.
		_ = os.Chtimes(name, now, now)
	}
	return b, nil
}

// deflate expands its input at most 1032 times. maxHint bounds how much
// gunzip allocates up front.
const (
	maxRatio = 1032
	maxHint  = 256 << 20
)

// readers holds *gzip.Reader for reuse.
var readers sync.Pool

// gunzip decompresses z. The reader checks the checksum at the end, so a
// damaged object fails.
func gunzip(z []byte) ([]byte, error) {
	zr, _ := readers.Get().(*gzip.Reader)
	var err error
	if zr == nil {
		zr, err = gzip.NewReader(bytes.NewReader(z))
	} else {
		err = zr.Reset(bytes.NewReader(z))
	}
	if err != nil {
		return nil, err
	}
	defer readers.Put(zr)
	zr.Multistream(false)
	// The trailer ends with the size of the content, modulo 2^32, so
	// usually the content is read without growing the buffer. A damaged
	// object may claim any size, so the hint is bounded by what deflate
	// can expand z to, and by a size larger than any object.
	var buf bytes.Buffer
	if n := len(z); n >= 4 {
		size := int64(binary.LittleEndian.Uint32(z[n-4:]))
		buf.Grow(int(min(size, int64(n)*maxRatio, maxHint)) + bytes.MinRead)
	}
	if _, err := buf.ReadFrom(zr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Put stores data as the object of kind named key, replacing any object of
// that name. Data that doesn't get smaller compressed is stored as is.
func (s *Store) Put(kind, key string, data []byte) error {
	if _, err := s.put(kind, key, data); err != nil {
		err = fmt.Errorf("put %s %s: %w", kind, key, err)
		s.failed("put", kind, err)
		return err
	}
	return nil
}

// Replace is Put without counting as a use: the object keeps the
// modification time of the one it replaces, if any, so that updating what
// an object says about itself, such as when it was last revalidated,
// doesn't keep it from being collected.
func (s *Store) Replace(kind, key string, data []byte) error {
	p, ok := s.path(kind, key)
	if !ok {
		return fmt.Errorf("replace %s %q: invalid name", kind, key)
	}
	var used time.Time
	for _, name := range []string{p + gzExt, p} {
		if fi, err := os.Stat(name); err == nil {
			used = fi.ModTime()
			break
		}
	}
	name, err := s.put(kind, key, data)
	if err != nil {
		err = fmt.Errorf("replace %s %s: %w", kind, key, err)
		s.failed("replace", kind, err)
		return err
	}
	if !used.IsZero() {
		// Only eviction depends on it, so a failure doesn't matter.
		_ = os.Chtimes(name, used, used)
	}
	return nil
}

// put stores data as the object of kind named key and returns the file it
// wrote.
func (s *Store) put(kind, key string, data []byte) (string, error) {
	p, ok := s.path(kind, key)
	if !ok {
		return "", errors.New("invalid name")
	}
	body, name, other := data, p, p+gzExt
	if s.compressed() {
		if z, err := s.gzip(data); err == nil && len(z) < len(data) {
			body, name, other = z, p+gzExt, p
		}
	}
	if err := writeFile(name, body); err != nil {
		return "", err
	}
	// An object of the other format would now be out of date. Most objects
	// never change, but some, such as what a branch points at, do.
	_ = os.Remove(other)
	return name, nil
}

func (s *Store) gzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(data) / 2)
	zw, _ := s.writers.Get().(*gzip.Writer)
	if zw == nil {
		var err error
		if zw, err = gzip.NewWriterLevel(&buf, s.opts.level); err != nil {
			return nil, err
		}
	} else {
		zw.Reset(&buf)
	}
	defer s.writers.Put(zw)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeFile writes data to a temporary file next to name and renames it to
// name, so that name is either the old object or the new one.
func writeFile(name string, data []byte) error {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	// CreateTemp makes the file readable by the user only.
	f, err := os.CreateTemp(dir, tmpPrefix+"*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, name)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// Delete removes the object of kind named key, if there is one.
func (s *Store) Delete(kind, key string) {
	p, ok := s.path(kind, key)
	if !ok {
		return
	}
	for _, name := range []string{p, p + gzExt} {
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			s.failed("delete", kind, err)
		}
	}
}

// failed counts a write of op, such as put, of an object of kind that
// failed with err, and logs it, with the home directory in its paths as
// ~. Callers go on without the disk, which is only a shortcut, so this is
// the one place that tells of it.
func (s *Store) failed(op, kind string, err error) {
	obs.CountCache(kind, obs.DiskWriteFailed)
	if ok, held := s.logs.Allow(op + " " + kind); ok {
		slog.Warn("disk cache write failed", append([]any{"span", "cache.disk", "kind", kind, "op", op,
			"err", obs.ShortHome(err.Error())}, obs.Suppressed(held)...)...)
	}
}

// dropped counts an object of kind that couldn't be read, for err, and
// is removed, and logs it.
func (s *Store) dropped(kind string, err error) {
	obs.CountCache(kind, obs.DiskDropped)
	if ok, held := s.logs.Allow("drop " + kind); ok {
		slog.Warn("kept dropped", append([]any{"span", "cache.disk", "kind", kind, "reason", "unreadable",
			"err", obs.ShortHome(err.Error())}, obs.Suppressed(held)...)...)
	}
}

func (s *Store) compressed() bool {
	return s.opts.level != gzip.NoCompression
}

// path returns the file of an object without its suffix. It reports false
// for a kind or key that isn't a plain name, so that no name leads out of
// the store.
func (s *Store) path(kind, key string) (string, bool) {
	if !validKind(kind) || !validKey(key) {
		return "", false
	}
	return filepath.Join(s.dir, kind, key[:2], key), true
}

// validKind reports whether kind is a word of lowercase letters.
func validKind(kind string) bool {
	if kind == "" || len(kind) > 32 {
		return false
	}
	for _, c := range []byte(kind) {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// validKey reports whether key is lowercase hex, at least 4 and at most 128
// digits long, like a hash.
func validKey(key string) bool {
	if len(key) < 4 || len(key) > 128 {
		return false
	}
	for _, c := range []byte(key) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
