package disk

import (
	"iter"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// List yields the name of every object of kind, with when it was last used:
// written, or read with Get. It reads the directories of kind as it goes,
// so objects that other processes add or remove meanwhile may be missed or
// yielded after all. Directories that can't be read are skipped.
func (s *Store) List(kind string) iter.Seq2[string, time.Time] {
	return func(yield func(string, time.Time) bool) {
		if !validKind(kind) {
			return
		}
		root := filepath.Join(s.dir, kind)
		dirs, err := os.ReadDir(root)
		if err != nil {
			return
		}
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(root, d.Name()))
			if err != nil {
				continue
			}
			// ReadDir sorts by name, so an object kept both compressed
			// and not, which Put avoids, is yielded once.
			var last string
			for _, f := range files {
				key := strings.TrimSuffix(f.Name(), gzExt)
				if f.IsDir() || key == last || !validKey(key) || !strings.HasPrefix(key, d.Name()) {
					continue
				}
				fi, err := f.Info()
				if err != nil {
					continue
				}
				last = key
				if !yield(key, fi.ModTime()) {
					return
				}
			}
		}
	}
}
