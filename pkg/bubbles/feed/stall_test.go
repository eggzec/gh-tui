package feed

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
)

// TestEmptyChunksStopTheWalk checks that pages that hold no item are read
// only emptyChunkWalk times ahead, then once more for each press down.
func TestEmptyChunksStopTheWalk(t *testing.T) {
	var calls atomic.Int32
	fetch := func(_ context.Context, _ string) ([]item, string, error) {
		n := int(calls.Add(1))
		return nil, strconv.Itoa(n), nil
	}
	m := newModel(fetch, renderItem, WithSize(40, 5), WithFocused(true))
	m = run(t, m, m.Init())
	if got := int(calls.Load()); got != emptyChunkWalk {
		t.Fatalf("fetches = %d, want %d", got, emptyChunkWalk)
	}
	if !m.stalled() {
		t.Fatal("not stalled after the empty chunks")
	}
	keys(t, m, "down")
	if got, want := int(calls.Load()), emptyChunkWalk+1; got != want {
		t.Fatalf("fetches after down = %d, want %d", got, want)
	}
}
