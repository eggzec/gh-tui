// Package cachetest helps test code that keeps cache entries in a Store.
package cachetest

import (
	"encoding/json"
	"iter"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Aged returns a view of store whose kept entries read as fetched d
// earlier than they were, as in a session that starts long after the one
// that kept them, so that they are stale however recently a test kept them.
// Objects that aren't kept entries read as they are. If store is a
// cache.Catalog, so is the view, so that a Shelf can still list what it
// keeps.
func Aged(store cache.Store, d time.Duration) cache.Store {
	a := aged{Store: store, d: d}
	if cat, ok := store.(cache.Catalog); ok {
		return agedCatalog{aged: a, cat: cat}
	}
	return a
}

// agedCatalog is the view of a cache.Catalog: an embedded cache.Store
// doesn't promote the Catalog's methods.
type agedCatalog struct {
	aged
	cat cache.Catalog
}

func (a agedCatalog) List(kind string) iter.Seq2[string, time.Time] {
	return a.cat.List(kind)
}

func (a agedCatalog) Peek(kind, key string) ([]byte, bool) {
	data, ok := a.cat.Peek(kind, key)
	if !ok {
		return nil, false
	}
	return a.age(data), true
}

func (a agedCatalog) Replace(kind, key string, data []byte) error {
	return a.cat.Replace(kind, key, data)
}

type aged struct {
	cache.Store
	d time.Duration
}

func (a aged) Get(kind, key string) ([]byte, bool) {
	data, ok := a.Store.Get(kind, key)
	if !ok {
		return nil, false
	}
	return a.age(data), true
}

// age returns data, a kept entry, as fetched a.d earlier.
func (a aged) age(data []byte) []byte {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return data
	}
	var at time.Time
	if json.Unmarshal(fields["fetched_at"], &at) != nil {
		return data
	}
	fields["fetched_at"], _ = json.Marshal(at.Add(-a.d))
	out, err := json.Marshal(fields)
	if err != nil {
		return data
	}
	return out
}
