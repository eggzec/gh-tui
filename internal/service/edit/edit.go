// Package edit holds the helpers that the pull and issue services share when
// they change what they cached of a comment, a label or an assignee.
package edit

import (
	"slices"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// Patch returns items with f applied to the first one that match accepts, on a
// copy, since the slice is shared with whoever holds the cached value. It
// reports false, and returns items, when none matches.
func Patch[T any](items []T, match func(T) bool, f func(*T)) ([]T, bool) {
	i := slices.IndexFunc(items, match)
	if i < 0 {
		return items, false
	}
	items = slices.Clone(items)
	f(&items[i])
	return items, true
}

// At returns when e was edited, or now if GitHub gave no time.
func At(e core.Edited, now time.Time) time.Time {
	if e.At.IsZero() {
		return now
	}
	return e.At
}

// OnTimeline returns the change of a cached timeline page that sets the body
// and update time of item id.
func OnTimeline(id, body string, at time.Time) func(core.Page[core.TimelineItem]) (core.Page[core.TimelineItem], bool) {
	return func(p core.Page[core.TimelineItem]) (core.Page[core.TimelineItem], bool) {
		items, ok := Patch(p.Items, func(it core.TimelineItem) bool { return it.ID == id }, func(it *core.TimelineItem) {
			it.Body, it.UpdatedAt = body, at
		})
		p.Items = items
		return p, ok
	}
}

// DropFromTimeline returns the change of a cached timeline page that removes
// item id.
func DropFromTimeline(id string) func(core.Page[core.TimelineItem]) (core.Page[core.TimelineItem], bool) {
	return func(p core.Page[core.TimelineItem]) (core.Page[core.TimelineItem], bool) {
		i := slices.IndexFunc(p.Items, func(it core.TimelineItem) bool { return it.ID == id })
		if i < 0 {
			return p, false
		}
		p.Items = slices.Delete(slices.Clone(p.Items), i, i+1)
		return p, true
	}
}

// Unkeep drops what the kept shelf holds under tag, so that a later session
// reads it again: the change it doesn't hold was only confirmed by GitHub's
// answer to the change, not by a read.
func Unkeep[V any](c *cache.Cache[V], shelf *cache.Shelf[V], tag string) {
	for key := range c.TaggedEntries(tag) {
		shelf.Delete(key)
	}
}

// Merged returns items without those whose name is in remove, which may differ
// in case, and with those of add that aren't there yet. It is a new slice,
// since the cache and the models share items.
func Merged[T any](items, add []T, remove []string, name func(T) string) []T {
	out := make([]T, 0, len(items)+len(add))
	for _, it := range items {
		if !slices.ContainsFunc(remove, func(n string) bool { return strings.EqualFold(n, name(it)) }) {
			out = append(out, it)
		}
	}
	for _, a := range add {
		if !slices.ContainsFunc(out, func(o T) bool { return strings.EqualFold(name(o), name(a)) }) {
			out = append(out, a)
		}
	}
	return out
}
