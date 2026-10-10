package pulls

import (
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	"context"

	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// fileSet holds the files of a pull request fetched so far, as the nodes of
// a tree of directories. The diff fetches them a page at a time, and the
// tree reads them in a command of its own, so the set is safe for both.
type fileSet struct {
	mu sync.RWMutex
	// minus is the sign before a count of deleted lines.
	minus string
	// kids are the children of each directory by its path, "" for the
	// top, and dirs the directories there are.
	kids map[string][]tree.Node
	dirs map[string]bool
	// files are the paths fetched, by their place in the diff.
	files map[string]int
}

func newFileSet(minus string) *fileSet {
	return &fileSet{minus: minus, kids: map[string][]tree.Node{}, dirs: map[string]bool{}, files: map[string]int{}}
}

// add adds a page of files, and returns the directories whose children
// changed, as the IDs of nodes, "" for the top: only those that no other
// holds, since reloading one reloads what is open below it. A file seen
// before is left as it was.
func (s *fileSet) add(files []diff.File) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	touched := map[string]bool{}
	for _, f := range files {
		if _, ok := s.files[f.Path]; ok || f.Path == "" {
			continue
		}
		s.files[f.Path] = len(s.files)
		dir, name := "", f.Path
		for {
			head, rest, more := strings.Cut(name, "/")
			if !more {
				break
			}
			id := path.Join(dir, head)
			if !s.dirs[id] {
				s.dirs[id] = true
				s.kids[dir] = append(s.kids[dir], tree.Node{ID: id, Name: head, Branch: true})
				touched[dir] = true
			}
			dir, name = id, rest
		}
		s.kids[dir] = append(s.kids[dir], tree.Node{ID: f.Path, Name: name, Detail: changes(f, s.minus), Value: f})
		touched[dir] = true
	}
	dirs := make([]string, 0, len(touched))
	for dir := range touched {
		dirs = append(dirs, dir)
		// Directories come first, as in the tree of the repository.
		slices.SortStableFunc(s.kids[dir], func(a, b tree.Node) int {
			if a.Branch != b.Branch {
				if a.Branch {
					return -1
				}
				return 1
			}
			return strings.Compare(a.Name, b.Name)
		})
	}
	slices.Sort(dirs)
	return slices.DeleteFunc(dirs, func(d string) bool {
		return slices.ContainsFunc(dirs, func(o string) bool {
			return o != d && (o == "" || strings.HasPrefix(d, o+"/"))
		})
	})
}

// children implements tree.Children.
func (s *fileSet) children(_ context.Context, parent tree.Node) ([]tree.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.kids[parent.ID]), nil
}

// index returns the place of the file at p in the diff, from 0, and false
// if it hasn't been fetched.
func (s *fileSet) index(p string) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.files[p]
	return i, ok
}

// list returns the files in the order of the diff.
func (s *fileSet) list() []diff.File {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]diff.File, len(s.files))
	for _, kids := range s.kids {
		for _, n := range kids {
			if f, ok := n.Value.(diff.File); ok {
				out[s.files[f.Path]] = f
			}
		}
	}
	return out
}

// len returns how many files there are.
func (s *fileSet) len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.files)
}

// changes is how many lines f adds and deletes, as "+12 −3", leaving out
// a count of none, or empty for a file with no counted lines.
func changes(f diff.File, minus string) string {
	var parts []string
	if f.Additions > 0 {
		parts = append(parts, "+"+strconv.Itoa(f.Additions))
	}
	if f.Deletions > 0 {
		parts = append(parts, minus+strconv.Itoa(f.Deletions))
	}
	return strings.Join(parts, " ")
}

// treePath returns the IDs of the nodes from the top of the tree down to
// the file at p, which a tree reveals.
func treePath(p string) []string {
	parts := strings.Split(p, "/")
	ids := make([]string, len(parts))
	for i := range parts {
		ids[i] = strings.Join(parts[:i+1], "/")
	}
	return ids
}
