// Package cachetest helps test code that keeps cache entries in a Store.
package cachetest

import (
	"encoding/json"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Aged returns a view of store whose kept entries read as fetched d
// earlier than they were, as in a session that starts long after the one
// that kept them, so that they are stale however recently a test kept them.
// Objects that aren't kept entries read as they are.
func Aged(store cache.Store, d time.Duration) cache.Store {
	return aged{Store: store, d: d}
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
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return data, true
	}
	var at time.Time
	if json.Unmarshal(fields["fetched_at"], &at) != nil {
		return data, true
	}
	fields["fetched_at"], _ = json.Marshal(at.Add(-a.d))
	out, err := json.Marshal(fields)
	if err != nil {
		return data, true
	}
	return out, true
}
