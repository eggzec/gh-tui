// Package logfile writes a log file that rotates by size, which several
// processes may append to at once.
//
// Each Write is one append with O_APPEND, so records written whole, such as
// the lines of a slog handler, never interleave. When the file outgrows its
// size, the process that notices renames it to path.1, path.1 to path.2,
// and so on, under a lock file, and the others follow to the new file
// within a second. Rotation is best effort: until they follow, the others
// still append to the renamed file, and a crashed process's lock is taken
// over once it is stale.
package logfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	// followEvery is how often a File checks whether another process
	// rotated the file.
	followEvery = time.Second
	// staleLock is how old a lock file is when its process is taken to
	// have died holding it.
	staleLock = 10 * time.Second
)

// File is a log file that rotates once it grows past its size, keeping a
// number of old files. It is safe for concurrent use.
type File struct {
	path    string
	maxSize int64
	keep    int

	mu       sync.Mutex
	f        *os.File
	size     int64
	followed time.Time
}

// Open opens the log file at path for appending, creating it and its
// directory if needed, readable by the user only, since logs name private
// repositories. The file rotates once it would grow past maxSize bytes,
// and keep old files are kept.
func Open(path string, maxSize int64, keep int) (*File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	l := &File{path: path, maxSize: maxSize, keep: max(keep, 0)}
	if err := l.open(); err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	return l, nil
}

// Path returns the path of the log file.
func (l *File) Path() string { return l.path }

// Write appends p to the file in one write, first rotating the file if p
// would make it outgrow its size.
func (l *File) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return 0, fs.ErrClosed
	}
	if time.Since(l.followed) >= followEvery {
		l.follow()
	}
	if l.size > 0 && l.size+int64(len(p)) > l.maxSize {
		l.rotate()
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

// Close closes the file.
func (l *File) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// open opens the file at the path, in place of the one open. l.mu must be
// held, or l not shared yet.
func (l *File) open() error {
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	if l.f != nil {
		_ = l.f.Close()
	}
	l.f, l.size, l.followed = f, fi.Size(), time.Now()
	return nil
}

// follow opens the file at the path again if another process rotated the
// one open, and learns its size, which the other processes grow too. On
// failure it keeps the one open. l.mu must be held.
func (l *File) follow() {
	l.followed = time.Now()
	cur, err := l.f.Stat()
	if err != nil {
		return
	}
	if at, err := os.Stat(l.path); err == nil && os.SameFile(cur, at) {
		l.size = cur.Size()
		return
	}
	_ = l.open()
}

// rotate moves the file aside and opens a new one, unless another process
// is rotating it or did already, in which case it follows. l.mu must be
// held.
func (l *File) rotate() {
	unlock, ok := l.lock()
	if !ok {
		l.follow()
		return
	}
	defer unlock()
	cur, err := l.f.Stat()
	if err != nil {
		return
	}
	at, err := os.Stat(l.path)
	if err != nil || !os.SameFile(cur, at) {
		// Another process rotated it before this one took the lock.
		_ = l.open()
		return
	}
	if l.keep == 0 {
		_ = os.Remove(l.path)
	} else {
		_ = os.Remove(l.old(l.keep))
		for i := l.keep - 1; i >= 1; i-- {
			_ = os.Rename(l.old(i), l.old(i+1))
		}
		_ = os.Rename(l.path, l.old(1))
	}
	_ = l.open()
}

// old returns the path of the i-th old file, 1 the most recent.
func (l *File) old(i int) string {
	return l.path + "." + strconv.Itoa(i)
}

// lock takes the lock file of the path, taking over a stale one, and
// returns the function that releases it. It reports false if another
// process holds the lock.
func (l *File) lock() (unlock func(), ok bool) {
	name := l.path + ".lock"
	for range 2 {
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(name) }, true
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, false
		}
		fi, err := os.Stat(name)
		if err != nil || time.Since(fi.ModTime()) < staleLock {
			return nil, false
		}
		_ = os.Remove(name)
	}
	return nil, false
}
