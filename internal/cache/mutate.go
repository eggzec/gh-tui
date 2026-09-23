package cache

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
	prev, prevStale, prevVersion := n.entry, n.stale, n.version
	c.seq++
	n.entry.Value, n.version = fn(n.entry.Value), c.seq
	c.moveToFront(n)
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
		default:
			c.markStale(cur)
		}
	}, true
}
