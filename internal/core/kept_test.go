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
