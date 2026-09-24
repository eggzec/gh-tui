package cache

import "slices"

// Mutate replaces the value for key with fn(value) right away, for an
// optimistic update. It reports false, and does nothing, if key isn't cached.
//
// Call rollback if the change fails on the server. It restores the entry as
// it was before Mutate. If the entry was written or invalidated since, it
// invalidates the key instead, so newer data isn't overwritten with an old
// snapshot. Calling rollback again does nothing.
//
// fn runs with the cache locked, so it must not use the cache. It must return
// a new value rather than modify its argument, which rollback restores.
func (c *Cache[V]) Mutate(key string, fn func(V) V) (rollback func(), ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.items[key]
	if !ok {
		return func() {}, false
	}
	c.moveToFront(n)
	return c.mutate(key, n, fn(n.entry.Value)), true
}

// MutateTag applies fn to every entry tagged with tag, for an optimistic
// update that touches several entries, such as every cached page listing an
// issue. fn reports whether it changed the value; entries it leaves alone are
// not touched, so fetches in flight for them can still store their results.
//
// Rollback undoes every change like Mutate's rollback does. The rules for fn
// are the same as for Mutate.
func (c *Cache[V]) MutateTag(tag string, fn func(V) (V, bool)) (rollback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var rollbacks []func()
	for key, n := range c.items {
		if !slices.Contains(n.entry.Tags, tag) {
			continue
		}
		if v, changed := fn(n.entry.Value); changed {
			rollbacks = append(rollbacks, c.mutate(key, n, v))
		}
	}
	return func() {
		for _, r := range slices.Backward(rollbacks) {
			r()
		}
	}
}

// mutate replaces the value of n, the node of key, with v and returns its
// rollback. The caller holds c.mu.
func (c *Cache[V]) mutate(key string, n *node[V], v V) (rollback func()) {
	prev, prevStale, prevVersion := n.entry, n.stale, n.version
	c.seq++
	n.entry.Value, n.version = v, c.seq
	c.resize(n)
	version := n.version

	done := false
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if done {
			return
		}
		done = true
		cur, ok := c.items[key]
		switch {
		case !ok:
		case cur.version == version:
			// Restoring the old version too makes the entry exactly what it
			// was, so nested rollbacks unwind in order and a Fetch that
			// started before Mutate may still store its result.
			cur.entry, cur.stale, cur.version = prev, prevStale, prevVersion
			c.resize(cur)
		default:
			c.markStale(cur)
		}
	}
}
