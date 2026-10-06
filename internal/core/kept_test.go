package core

import (
	"context"
	"testing"
)

func TestWatchLimit(t *testing.T) {
	// Without a watch, nothing is told.
	ServedLimited(t.Context())

	ctx, limited := WatchLimit(t.Context())
	if limited() {
		t.Fatal("a watch with no read under it was told of a limit")
	}
	read, cancel := context.WithCancel(ctx)
	defer cancel()
	ServedLimited(read)
	if !limited() {
		t.Error("a read under the watch wasn't told to it")
	}
	if _, other := WatchLimit(t.Context()); other() {
		t.Error("another watch was told of the read")
	}
}

// TestWatchLimitNested checks that a watch within another tells the outer
// one too, and that a read under the outer one only tells the outer.
func TestWatchLimitNested(t *testing.T) {
	outer, outerLimited := WatchLimit(t.Context())
	inner, innerLimited := WatchLimit(outer)
	ServedLimited(inner)
	if !innerLimited() || !outerLimited() {
		t.Errorf("inner watch told %v, outer %v; want both", innerLimited(), outerLimited())
	}

	outer, outerLimited = WatchLimit(t.Context())
	_, innerLimited = WatchLimit(outer)
	ServedLimited(outer)
	if innerLimited() || !outerLimited() {
		t.Errorf("inner watch told %v, outer %v; want only the outer", innerLimited(), outerLimited())
	}
}
