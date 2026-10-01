package disk

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// tmpPrefix starts the names of files being written. Collect removes old
// ones, which a process that died while writing left behind.
const tmpPrefix = ".tmp-"

// tmpMaxAge is how old a temporary file is before Collect takes it for
// abandoned. No write takes nearly that long.
const tmpMaxAge = time.Hour

// Usage is what Collect found in the store and what it removed.
type Usage struct {
	// Files and Size count the objects that remain, and their size in
	// bytes on disk.
	Files int
	Size  int64
	// Removed counts the objects and abandoned files that Collect removed.
	Removed int
}

// Collect removes the objects used least recently, by modification time,
// until the store is below its maximum size, and removes abandoned
// temporary files. Once over the limit it goes down to nine tenths of it,
// so the next sessions don't collect again right away. It walks the whole
// store, so run it in the background. It stops when ctx is done.
func (s *Store) Collect(ctx context.Context) (Usage, error) {
	type object struct {
		path  string
		size  int64
		mtime time.Time
	}
	var (
		objects []object
		u       Usage
	)
	now := time.Now()
	err := filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Another process may have removed it meanwhile.
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if slices.Contains(s.opts.keep, d.Name()) {
			return nil
		}
		if strings.HasPrefix(d.Name(), tmpPrefix) {
			if now.Sub(fi.ModTime()) > tmpMaxAge && os.Remove(p) == nil {
				u.Removed++
			}
			return nil
		}
		objects = append(objects, object{path: p, size: fi.Size(), mtime: fi.ModTime()})
		u.Size += fi.Size()
		return nil
	})
	if err != nil {
		return u, fmt.Errorf("collect disk cache: %w", err)
	}
	u.Files = len(objects)
	if s.opts.maxSize < 1 || u.Size <= s.opts.maxSize {
		return u, nil
	}

	slices.SortFunc(objects, func(a, b object) int { return a.mtime.Compare(b.mtime) })
	target := s.opts.maxSize / 10 * 9
	for _, o := range objects {
		if u.Size <= target {
			break
		}
		if err := ctx.Err(); err != nil {
			return u, fmt.Errorf("collect disk cache: %w", err)
		}
		// If another process removed it first, the space is free all the
		// same.
		if err := os.Remove(o.path); err == nil || os.IsNotExist(err) {
			u.Size -= o.size
			u.Files--
			u.Removed++
		}
	}
	return u, nil
}
