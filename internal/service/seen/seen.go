// Package seen records the version of each item, such as an issue's update
// time, that a list last showed, so that a cached detail of the item can be
// shown without asking GitHub while the list vouches that it hasn't changed.
package seen

import (
	"strings"
	"sync"
	"time"
)

// Marks holds a mark of type M for each item key. The zero value is ready to
// use, and it is safe for concurrent use.
type Marks[M any] struct {
	mu sync.Mutex
	m  map[string]M
}

// Set records m for key, replacing the previous mark, and returns that one.
func (s *Marks[M]) Set(key string, m M) (prev M, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]M)
	}
	prev, ok = s.m[key]
	s.m[key] = m
	return prev, ok
}

// Get returns the mark of key.
func (s *Marks[M]) Get(key string) (M, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.m[key]
	return m, ok
}

// Delete forgets the mark of key.
func (s *Marks[M]) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

// DeletePrefix forgets the marks of every key that starts with prefix, such
// as every item of a repository.
func (s *Marks[M]) DeletePrefix(prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			delete(s.m, k)
		}
	}
}

// Stamped is a cached value, such as a page of comments, stamped with the
// version of the item it belongs to when it was fetched. A zero Version
// means the version wasn't known.
type Stamped[V any] struct {
	Value   V
	Version time.Time
}

// Current reports whether something at version v is at least as recent as
// mark, the version a list showed. Nothing is current without a version.
func Current(v, mark time.Time) bool {
	return !v.IsZero() && !mark.IsZero() && !v.Before(mark)
}
